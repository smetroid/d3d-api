package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/dgrijalva/jwt-go"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo"
	"github.com/smetroid/d3d-api/app/collab"
	"github.com/smetroid/d3d-api/app/db/postgres"
	"github.com/smetroid/d3d-api/app/models"
	"github.com/smetroid/d3d-api/app/services"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type DAGsController struct {
	Echo           *echo.Echo
	DAGService     services.DAGService
	Hub            *collab.Hub
	DB             *postgres.Postgres
	AuthMiddleware echo.MiddlewareFunc
	LogDAGRequests bool
}

func (dc *DAGsController) Init() {
	dc.Echo.POST("/dag", dc.createDAG, dc.AuthMiddleware)
	dc.Echo.POST("/dag/:dag/update", dc.updateDAG, dc.AuthMiddleware)
	dc.Echo.GET("/dags", dc.getDAGs, dc.AuthMiddleware)
	dc.Echo.GET("/dag/:dag", dc.getDAG, dc.AuthMiddleware)
	dc.Echo.DELETE("/dag/:dag", dc.deleteDAG, dc.AuthMiddleware)
	dc.Echo.GET("/dag/:dag/ws", dc.dagWS, dc.AuthMiddleware)
	dc.Echo.GET("/dag/:dag/history", dc.getDAGHistory, dc.AuthMiddleware)
	dc.Echo.POST("/dag/:dag/history/:historyId/restore", dc.restoreDAGHistory, dc.AuthMiddleware)
	dc.Echo.PATCH("/dag/:dag", dc.setPublic, dc.AuthMiddleware)
	dc.Echo.GET("/dag/:dag/public", dc.getDAGPublic)
}

func (dc *DAGsController) createDAG(ctx echo.Context) error {
	// Commenting the lines below fixes the EOF error
	// request, _ := ioutil.ReadAll(ctx.Request().Body)
	// log.Println("Dag Request")
	// log.Println(string(request))

	var incomingDag models.Dag
	err := ctx.Bind(&incomingDag)
	if err != nil {
		log.Println("ctx.Binding Error Found in DAG")
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse(err.Error()))
	}

	dagsResponse, err := dc.DAGService.ProcessDAG(incomingDag)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}

	return ctx.JSON(http.StatusCreated, dagsResponse)
}

func (dc *DAGsController) getDAGs(ctx echo.Context) error {
	ctx.QueryParams()
	dagsResponse, err := dc.DAGService.GetDAGs(ctx.QueryParams())
	return dc.StandardResponse(ctx, dagsResponse, err)

}

func (dc *DAGsController) getDAG(ctx echo.Context) error {
	dagResponse, err := dc.DAGService.GetDAG(ctx.Param("dag"))
	if errors.Is(err, postgres.ErrNotFound) {
		return ctx.JSON(http.StatusNotFound, models.ErrorResponse("diagram not found"))
	}
	return dc.StandardResponse(ctx, dagResponse, err)
}

func (dc *DAGsController) deleteDAG(ctx echo.Context) error {
	// Share tokens with view role cannot write.
	if _, role, isShare := shareInfoFromCtx(ctx); isShare && role != "edit" {
		return ctx.JSON(http.StatusForbidden, models.ErrorResponse("view-only share link"))
	}

	err := dc.DAGService.DeleteDAG(ctx.Param("dag"))
	return dc.StandardResponse(ctx, struct {
		Status string `json:"status"`
	}{Status: "ok"}, err)
}

func (dc *DAGsController) StandardResponse(ctx echo.Context, response interface{}, err error) error {
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	return ctx.JSON(http.StatusOK, response)
}

func (dc *DAGsController) updateDAG(ctx echo.Context) error {
	// Share tokens with view role cannot write.
	if _, role, isShare := shareInfoFromCtx(ctx); isShare && role != "edit" {
		return ctx.JSON(http.StatusForbidden, models.ErrorResponse("view-only share link"))
	}

	var dagUpdate models.Dag

	// request, _ := ioutil.ReadAll(ctx.Request().Body)
	// log.Println(string(request))

	err := ctx.Bind(&dagUpdate)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse(err.Error()))
	}

	dagId := ctx.Param("dag")
	err = dc.DAGService.UpdateDAG(dagId, dagUpdate)
	if errors.Is(err, postgres.ErrNotFound) {
		return ctx.JSON(http.StatusNotFound, models.ErrorResponse("diagram not found"))
	}
	if err != nil {
		return dc.StandardResponse(ctx, models.OK_RESPONSE, err)
	}

	// Append snapshot to history (non-blocking).
	if dagUpdate.Diagram != "" {
		savedBy := usernameFromCtx(ctx)
		go dc.DAGService.AppendHistory(dagId, dagUpdate.Diagram, savedBy) //nolint:errcheck
	}

	// Broadcast diagram:updated to all WS peers in this room.
	if dc.Hub != nil {
		msg, _ := json.Marshal(map[string]interface{}{
			"type":     "diagram:updated",
			"dagId":    dagId,
			"clientId": dagUpdate.ClientId,
		})
		dc.Hub.Broadcast(dagId, msg, nil)
	}

	return dc.StandardResponse(ctx, models.OK_RESPONSE, nil)
}

// dagWS upgrades a GET /dag/:dag/ws request to a WebSocket connection and
// adds the client to the collab hub for the requested diagram room.
func (dc *DAGsController) dagWS(ctx echo.Context) error {
	dagId := ctx.Param("dag")

	// Reject revoked share tokens before upgrading.
	if jti, _, isShare := shareInfoFromCtx(ctx); isShare && dc.DB != nil {
		revoked, err := dc.DB.IsRevoked(jti)
		if err != nil || revoked {
			return ctx.JSON(http.StatusForbidden, models.ErrorResponse("share link revoked"))
		}
	}

	conn, err := wsUpgrader.Upgrade(ctx.Response(), ctx.Request(), nil)
	if err != nil {
		log.Printf("ws upgrade error dag=%s: %v", dagId, err)
		return nil
	}

	client := &collab.Client{
		DagId: dagId,
		Conn:  conn,
		Send:  make(chan []byte, 64),
	}
	dc.Hub.Join(client)
	defer dc.Hub.Leave(client)

	go client.WritePump()

	// Read loop — relay presence messages from this client to room peers.
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var envelope map[string]interface{}
		if json.Unmarshal(msg, &envelope) != nil {
			continue
		}
		if envelope["type"] == "presence" {
			dc.Hub.Broadcast(dagId, msg, client)
		}
	}
	return nil
}

func (dc *DAGsController) getDAGHistory(ctx echo.Context) error {
	dagId := ctx.Param("dag")
	resp, err := dc.DAGService.GetHistory(dagId)
	return dc.StandardResponse(ctx, resp, err)
}

func (dc *DAGsController) restoreDAGHistory(ctx echo.Context) error {
	// Share tokens with view role cannot write.
	if _, role, isShare := shareInfoFromCtx(ctx); isShare && role != "edit" {
		return ctx.JSON(http.StatusForbidden, models.ErrorResponse("view-only share link"))
	}

	dagId := ctx.Param("dag")
	historyId := ctx.Param("historyId")

	err := dc.DAGService.RestoreHistory(historyId, dagId)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}

	// Broadcast so live peers see the restored state.
	if dc.Hub != nil {
		msg, _ := json.Marshal(map[string]interface{}{
			"type":  "diagram:updated",
			"dagId": dagId,
		})
		dc.Hub.Broadcast(dagId, msg, nil)
	}

	return ctx.JSON(http.StatusOK, models.OK_RESPONSE)
}

// shareInfoFromCtx returns jti/role from a share token; isShare is false for regular auth tokens.
func shareInfoFromCtx(ctx echo.Context) (jti, role string, isShare bool) {
	u, ok := ctx.Get("user").(*jwt.Token)
	if !ok || u == nil {
		return
	}
	claims, ok := u.Claims.(jwt.MapClaims)
	if !ok {
		return
	}
	iss, _ := claims["iss"].(string)
	if iss != "d3d-share" {
		return
	}
	jti, _ = claims["jti"].(string)
	role, _ = claims["role"].(string)
	isShare = true
	return
}

// usernameFromCtx extracts the "jti" claim (username) from the JWT stored
// in the Echo context by the auth middleware.
func usernameFromCtx(ctx echo.Context) string {
	u, ok := ctx.Get("user").(*jwt.Token)
	if !ok || u == nil {
		return ""
	}
	claims, ok := u.Claims.(jwt.MapClaims)
	if !ok {
		return ""
	}
	jti, _ := claims["jti"].(string)
	return jti
}

// setPublic toggles the public embed flag for a DAG. Auth required (owner only).
// Body: {"public": true|false}
func (dc *DAGsController) setPublic(ctx echo.Context) error {
	// Share tokens with view role cannot write.
	if _, role, isShare := shareInfoFromCtx(ctx); isShare && role != "edit" {
		return ctx.JSON(http.StatusForbidden, models.ErrorResponse("view-only share link"))
	}

	dagId := ctx.Param("dag")
	var body struct {
		Public bool `json:"public"`
	}
	if err := ctx.Bind(&body); err != nil {
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse(err.Error()))
	}
	if err := dc.DAGService.SetPublic(dagId, body.Public); err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	return ctx.JSON(http.StatusOK, models.OK_RESPONSE)
}

// getDAGPublic returns the diagram model if the DAG has Public == true.
// No authentication required. Returns 404 for non-existent or non-public diagrams.
func (dc *DAGsController) getDAGPublic(ctx echo.Context) error {
	dagId := ctx.Param("dag")
	dag, err := dc.DAGService.GetDAGPublic(dagId)
	if err != nil {
		return ctx.JSON(http.StatusNotFound, models.ErrorResponse("not found"))
	}
	ctx.Response().Header().Set("ETag", fmt.Sprintf(`"%d"`, dag.EmbedRevision))
	ctx.Response().Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=600")
	ctx.Response().Header().Set("Access-Control-Allow-Origin", "*")
	return ctx.JSON(http.StatusOK, models.NewDagPublicResponse(dag))
}

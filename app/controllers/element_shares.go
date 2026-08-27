package controllers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"encoding/json"
	jwt "github.com/dgrijalva/jwt-go"
	"github.com/google/uuid"
	"github.com/labstack/echo"
	"github.com/smetroid/d3d-api/app/auth/membership"
	"github.com/smetroid/d3d-api/app/auth/token"
	"github.com/smetroid/d3d-api/app/cluster"
	"github.com/smetroid/d3d-api/app/db/postgres"
	"github.com/smetroid/d3d-api/app/models"
)

type ElementSharesController struct {
	Echo           *echo.Echo
	DB             *postgres.Postgres
	SigningKey     string
	AuthMiddleware echo.MiddlewareFunc
}

func (ec *ElementSharesController) Init() {
	// Public — no auth required
	ec.Echo.GET("/element-shares/exchange", ec.exchangeElementShare)
	ec.Echo.GET("/catalog", ec.listCatalog)
	// Auth required
	ec.Echo.POST("/dag/:dag/elements/shares", ec.createElementShare, ec.AuthMiddleware)
	ec.Echo.GET("/element-shares/:id", ec.getElementShare, ec.AuthMiddleware)
	ec.Echo.POST("/element-shares/:id/revoke", ec.revokeElementShare, ec.AuthMiddleware)
	ec.Echo.POST("/element-shares/:id/import", ec.importElementShare, ec.AuthMiddleware)
	ec.Echo.GET("/shares/inbox", ec.listInbox, ec.AuthMiddleware)
}

// POST /dag/:dag/elements/shares
func (ec *ElementSharesController) createElementShare(ctx echo.Context) error {
	dagId := ctx.Param("dag")
	caller := usernameFromCtx(ctx)

	var req models.CreateElementShareRequest
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse(err.Error()))
	}
	if len(req.RootIds) == 0 {
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse("rootIds is required"))
	}
	if req.Audience.Kind == "" {
		req.Audience.Kind = "public"
	}
	if req.Role != "view" && req.Role != "edit" {
		req.Role = "view"
	}
	if req.ExpDays <= 0 {
		req.ExpDays = 7
	}

	// Fetch the diagram to compute the cluster server-side.
	dag, err := ec.DB.GetDAG(dagId)
	if err == postgres.ErrNotFound {
		return ctx.JSON(http.StatusNotFound, models.ErrorResponse("diagram not found"))
	}
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	if dag.Diagram == "" {
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse("diagram has no content"))
	}

	// Validate that all rootIds exist in the diagram.
	missing, err := cluster.ValidateRootIds(dag.Diagram, req.RootIds)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse("invalid diagram JSON: "+err.Error()))
	}
	if len(missing) > 0 {
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse(
			"rootIds not found in diagram: "+strings.Join(missing, ", ")))
	}

	// Compute cluster closure.
	depth := 0
	if req.Depth != nil {
		depth = *req.Depth
	} else {
		depth = -1 // nil → whole connected component
	}
	subgraph, err := cluster.Compute(dag.Diagram, req.RootIds, depth)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse("cluster computation failed: "+err.Error()))
	}
	clusterJSON, err := json.Marshal(subgraph)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}

	// Determine element type from rootIds.
	elemType := detectType(dag.Diagram, req.RootIds)

	exp := time.Now().Add(time.Duration(req.ExpDays) * 24 * time.Hour)
	share := models.ElementShare{
		Id:           uuid.New().String(),
		Title:        req.Title,
		Type:         elemType,
		RootIds:      req.RootIds,
		Cluster:      string(clusterJSON),
		AudienceKind: req.Audience.Kind,
		AudienceIds:  req.Audience.Ids,
		Role:         req.Role,
		CreatedBy:    caller,
		SourceDagId:  dagId,
		ExpiresAt:    exp,
		Catalog:      req.Catalog,
		Tags:         req.Tags,
		ImportedBy:   []string{},
		AnonName:     randomAnonName(),
		CreatedAt:    time.Now(),
	}
	if share.Tags == nil {
		share.Tags = []string{}
	}
	if share.AudienceIds == nil {
		share.AudienceIds = []string{}
	}

	var shareToken string
	if req.Audience.Kind == "public" {
		jti := uuid.New().String()
		share.Jti = jti
		claims := jwt.MapClaims{
			"jti":      jti,
			"iss":      "d3d-element-share",
			"share_id": share.Id,
			"role":     req.Role,
			"exp":      exp.Unix(),
		}
		shareToken, err = token.CreateToken(ec.SigningKey, claims)
		if err != nil {
			return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
		}
	}

	id, err := ec.DB.CreateElementShare(share)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}

	resp := map[string]interface{}{
		"status": "ok",
		"id":     id,
	}
	if shareToken != "" {
		resp["token"] = shareToken
	}
	return ctx.JSON(http.StatusCreated, resp)
}

// GET /element-shares/exchange?token=<jwt>  (no auth)
func (ec *ElementSharesController) exchangeElementShare(ctx echo.Context) error {
	raw := ctx.QueryParam("token")
	if raw == "" {
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse("missing token"))
	}

	tok, err := jwt.Parse(raw, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(ec.SigningKey), nil
	})
	if err != nil || !tok.Valid {
		return ctx.JSON(http.StatusUnauthorized, models.ErrorResponse("invalid or expired token"))
	}

	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return ctx.JSON(http.StatusUnauthorized, models.ErrorResponse("invalid claims"))
	}
	if iss, _ := claims["iss"].(string); iss != "d3d-element-share" {
		return ctx.JSON(http.StatusUnauthorized, models.ErrorResponse("not an element share token"))
	}

	jti, _ := claims["jti"].(string)
	shareId, _ := claims["share_id"].(string)
	role, _ := claims["role"].(string)

	revoked, err := ec.DB.IsRevoked(jti)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	if revoked {
		return ctx.JSON(http.StatusForbidden, models.ErrorResponse("share link revoked"))
	}

	share, err := ec.DB.GetElementShare(shareId)
	if err == postgres.ErrNotFound {
		return ctx.JSON(http.StatusNotFound, models.ErrorResponse("share not found"))
	}
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	if share.Revoked {
		return ctx.JSON(http.StatusForbidden, models.ErrorResponse("share revoked"))
	}

	return ctx.JSON(http.StatusOK, map[string]interface{}{
		"status":   "ok",
		"shareId":  share.Id,
		"title":    share.Title,
		"role":     role,
		"anonName": share.AnonName,
		"cluster":  json.RawMessage(share.Cluster),
		"type":     share.Type,
		"rootIds":  share.RootIds,
	})
}

// GET /element-shares/:id
func (ec *ElementSharesController) getElementShare(ctx echo.Context) error {
	id := ctx.Param("id")
	caller := usernameFromCtx(ctx)

	share, err := ec.DB.GetElementShare(id)
	if err == postgres.ErrNotFound {
		return ctx.JSON(http.StatusNotFound, models.ErrorResponse("share not found"))
	}
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	if share.Revoked {
		return ctx.JSON(http.StatusGone, models.ErrorResponse("share revoked"))
	}

	// Access check for non-public shares.
	if share.AudienceKind != "public" {
		allowed, err := membership.UserInAudience(ec.DB, caller, models.AudienceSpec{
			Kind: share.AudienceKind,
			Ids:  share.AudienceIds,
		})
		if err != nil {
			return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
		}
		if !allowed && share.CreatedBy != caller {
			return ctx.JSON(http.StatusForbidden, models.ErrorResponse("access denied"))
		}
	}

	return ctx.JSON(http.StatusOK, map[string]interface{}{
		"id":           share.Id,
		"title":        share.Title,
		"type":         share.Type,
		"rootIds":      share.RootIds,
		"cluster":      json.RawMessage(share.Cluster),
		"audienceKind": share.AudienceKind,
		"audienceIds":  share.AudienceIds,
		"role":         share.Role,
		"createdBy":    share.CreatedBy,
		"sourceDagId":  share.SourceDagId,
		"expiresAt":    share.ExpiresAt,
		"revoked":      share.Revoked,
		"catalog":      share.Catalog,
		"tags":         share.Tags,
		"importedBy":   share.ImportedBy,
		"jti":          share.Jti,
		"anonName":     share.AnonName,
		"createdAt":    share.CreatedAt,
	})
}

// POST /element-shares/:id/revoke
func (ec *ElementSharesController) revokeElementShare(ctx echo.Context) error {
	id := ctx.Param("id")
	caller := usernameFromCtx(ctx)

	share, err := ec.DB.GetElementShare(id)
	if err == postgres.ErrNotFound {
		return ctx.JSON(http.StatusNotFound, models.ErrorResponse("share not found"))
	}
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	if share.CreatedBy != caller {
		return ctx.JSON(http.StatusForbidden, models.ErrorResponse("only the owner can revoke"))
	}

	if err := ec.DB.RevokeElementShare(id); err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	// Also denylist the JWT if this was a public link share.
	if share.Jti != "" {
		_ = ec.DB.RevokeShare(models.Share{Jti: share.Jti}.Jti)
	}
	return ctx.JSON(http.StatusOK, models.OK_RESPONSE)
}

// POST /element-shares/:id/import
func (ec *ElementSharesController) importElementShare(ctx echo.Context) error {
	id := ctx.Param("id")
	caller := usernameFromCtx(ctx)

	share, err := ec.DB.GetElementShare(id)
	if err == postgres.ErrNotFound {
		return ctx.JSON(http.StatusNotFound, models.ErrorResponse("share not found"))
	}
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	if share.Revoked {
		return ctx.JSON(http.StatusGone, models.ErrorResponse("share revoked"))
	}

	if share.CreatedBy != caller {
		allowed, err := membership.UserInAudience(ec.DB, caller, models.AudienceSpec{
			Kind: share.AudienceKind,
			Ids:  share.AudienceIds,
		})
		if err != nil {
			return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
		}
		if !allowed {
			return ctx.JSON(http.StatusForbidden, models.ErrorResponse("access denied"))
		}
	}

	if err := ec.DB.AppendImportedBy(id, caller); err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	return ctx.JSON(http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"cluster": json.RawMessage(share.Cluster),
		"type":    share.Type,
		"rootIds": share.RootIds,
	})
}

// GET /shares/inbox
func (ec *ElementSharesController) listInbox(ctx echo.Context) error {
	caller := usernameFromCtx(ctx)

	companyIds, err := ec.DB.GetUserCompanyIds(caller)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	groupIds, err := ec.DB.GetUserGroupIds(caller)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}

	shares, err := ec.DB.ListInboxShares(caller, companyIds, groupIds)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	return ctx.JSON(http.StatusOK, map[string]interface{}{
		"status": "ok",
		"shares": shares,
	})
}

// GET /catalog  (public, no auth)
func (ec *ElementSharesController) listCatalog(ctx echo.Context) error {
	limit := 50
	if l := ctx.QueryParam("limit"); l != "" {
		if n, err := fmt.Sscanf(l, "%d", &limit); n != 1 || err != nil || limit <= 0 {
			limit = 50
		}
		if limit > 200 {
			limit = 200
		}
	}

	rows, err := ec.DB.ListCatalogShares(limit)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}

	items := make([]models.CatalogEntry, 0, len(rows))
	for _, r := range rows {
		nodeCount, edgeCount := countClusterElements(r.Cluster)

		var tok string
		if r.Jti != "" {
			exp := r.ExpiresAt.Unix()
			if r.ExpiresAt.IsZero() {
				exp = 0
			}
			claims := jwt.MapClaims{
				"jti":      r.Jti,
				"iss":      "d3d-element-share",
				"share_id": r.Id,
				"role":     "view",
			}
			if exp > 0 {
				claims["exp"] = exp
			}
			tok, _ = token.CreateToken(ec.SigningKey, claims)
		}

		items = append(items, models.CatalogEntry{
			Id:        r.Id,
			Title:     r.Title,
			CreatedBy: r.CreatedBy,
			RootIds:   r.RootIds,
			NodeCount: nodeCount,
			EdgeCount: edgeCount,
			Token:     tok,
			Tags:      r.Tags,
			ExpiresAt: r.ExpiresAt,
			CreatedAt: r.CreatedAt,
		})
	}

	return ctx.JSON(http.StatusOK, map[string]interface{}{
		"status": "ok",
		"items":  items,
	})
}

func countClusterElements(clusterJSON string) (nodeCount, edgeCount int) {
	var g struct {
		Nodes []json.RawMessage `json:"nodes"`
		Edges []json.RawMessage `json:"edges"`
	}
	_ = json.Unmarshal([]byte(clusterJSON), &g)
	return len(g.Nodes), len(g.Edges)
}

// detectType returns "node", "edge", or "cluster" based on whether rootIds
// refer to nodes or edges in the diagram JSON (best-effort; defaults to "cluster").
func detectType(diagramJSON string, rootIds []string) string {
	if len(rootIds) == 0 {
		return "cluster"
	}
	var wrapper struct {
		Nodes []struct {
			V string `json:"v"`
		} `json:"nodes"`
		Edges []struct {
			Value map[string]interface{} `json:"value"`
		} `json:"edges"`
	}
	_ = json.Unmarshal([]byte(diagramJSON), &wrapper)
	nodeSet := make(map[string]bool, len(wrapper.Nodes))
	for _, n := range wrapper.Nodes {
		nodeSet[n.V] = true
	}
	if len(rootIds) == 1 && nodeSet[rootIds[0]] {
		return "node"
	}
	if len(rootIds) > 1 {
		return "cluster"
	}
	return "node"
}

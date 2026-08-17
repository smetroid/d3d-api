package controllers

import (
	"net/http"

	"github.com/labstack/echo"
	"github.com/smetroid/d3d-api/app/models"
	"github.com/smetroid/d3d-api/app/services"
)

type NodeController struct {
	Echo            *echo.Echo
	NodeService     services.NodeService
	AuthMiddleware  echo.MiddlewareFunc
	LogNodeRequests bool
}

func (dc *NodeController) Init() {
	dc.Echo.POST("/node", dc.createNode, dc.AuthMiddleware)
	dc.Echo.GET("/nodes", dc.getNodes, dc.AuthMiddleware)

}

func (dc *NodeController) createNode(ctx echo.Context) error {
	// request, _ := ioutil.ReadAll(ctx.Request().Body)
	// log.Println(string(request))

	var incomingNode models.Node
	err := ctx.Bind(&incomingNode)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse(err.Error()))
	}

	nodesResponse, err := dc.NodeService.ProcessNode(incomingNode)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}

	return ctx.JSON(http.StatusCreated, nodesResponse)
}

func (dc *NodeController) getNodes(ctx echo.Context) error {
	ctx.QueryParams()
	nodesResponse, err := dc.NodeService.GetNodes(ctx.QueryParams())
	return dc.StandardResponse(ctx, nodesResponse, err)

}

func (ac *NodeController) StandardResponse(ctx echo.Context, response interface{}, err error) error {
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	return ctx.JSON(http.StatusOK, response)
}

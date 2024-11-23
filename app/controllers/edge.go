package controllers

import (
	"net/http"

	"github.com/labstack/echo"
	"github.com/smetroid/d3d-api/app/models"
	"github.com/smetroid/d3d-api/app/services"
)

type EdgeController struct {
	Echo            *echo.Echo
	EdgeService     services.EdgeService
	AuthMiddleware  echo.MiddlewareFunc
	LogEdgeRequests bool
}

func (dc *EdgeController) Init() {
	dc.Echo.POST("/edge", dc.createEdge, dc.AuthMiddleware)
	dc.Echo.GET("/edges", dc.getEdges, dc.AuthMiddleware)

}

func (dc *EdgeController) createEdge(ctx echo.Context) error {
	if dc.LogEdgeRequests {
		// Commenting this out causing EOF errors... ctx.REquest can only be read once
		//request, _ := ioutil.ReadAll(ctx.Request().Body)
		//log.Println(string(request))
	}

	var incomingEdge models.Edge
	err := ctx.Bind(&incomingEdge)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse(err.Error()))
	}

	edgesResponse, err := dc.EdgeService.ProcessEdge(incomingEdge)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}

	return ctx.JSON(http.StatusCreated, edgesResponse)
	return nil
}

func (dc *EdgeController) getEdges(ctx echo.Context) error {
	ctx.QueryParams()
	edgesResponse, err := dc.EdgeService.GetEdges(ctx.QueryParams())
	return dc.StandardResponse(ctx, edgesResponse, err)

}

func (dc *EdgeController) getEdge(ctx echo.Context) error {
	edgeResponse, err := dc.EdgeService.GetEdge(ctx.Param("edges"))
	return dc.StandardResponse(ctx, edgeResponse, err)
}

func (dc *EdgeController) deleteEdge(ctx echo.Context) error {
	err := dc.EdgeService.DeleteEdge(ctx.Param("edge"))
	return dc.StandardResponse(ctx, struct {
		Status string `json:"status"`
	}{Status: "ok"}, err)
}

func (ac *EdgeController) StandardResponse(ctx echo.Context, response interface{}, err error) error {
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	return ctx.JSON(http.StatusOK, response)
}

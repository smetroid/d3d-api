package controllers

import (
	"log"
	"net/http"

	"github.com/labstack/echo"
	"github.com/smetroid/d3d-api/app/models"
	"github.com/smetroid/d3d-api/app/services"
)

type DAGsController struct {
	Echo           *echo.Echo
	DAGService     services.DAGService
	AuthMiddleware echo.MiddlewareFunc
	LogDAGRequests bool
}

func (dc *DAGsController) Init() {
	dc.Echo.POST("/dag", dc.createDAG, dc.AuthMiddleware)
	dc.Echo.POST("/dag/:dag/update", dc.updateDAG, dc.AuthMiddleware)
	dc.Echo.GET("/dags", dc.getDAGs, dc.AuthMiddleware)
	dc.Echo.GET("/dag/:dag", dc.getDAG, dc.AuthMiddleware)
	dc.Echo.DELETE("/dag/:dag", dc.deleteDAG, dc.AuthMiddleware)

}

func (dc *DAGsController) createDAG(ctx echo.Context) error {

	if dc.LogDAGRequests {
		// Commenting the lines below fixes the EOF error
		// request, _ := ioutil.ReadAll(ctx.Request().Body)
		// log.Println("Dag Request")
		// log.Println(string(request))
	}

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
	return dc.StandardResponse(ctx, dagResponse, err)
}

func (dc *DAGsController) deleteDAG(ctx echo.Context) error {
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
	var dagUpdate models.Dag

	if dc.LogDAGRequests {
		// request, _ := ioutil.ReadAll(ctx.Request().Body)
		// log.Println(string(request))
	}

	err := ctx.Bind(&dagUpdate)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse(err.Error()))
	}

	err = dc.DAGService.UpdateDAG(ctx.Param("dag"), dagUpdate)
	return dc.StandardResponse(ctx, models.OK_RESPONSE, err)
}

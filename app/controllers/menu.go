package controllers

import (
	"net/http"

	"github.com/labstack/echo"
	"github.com/smetroid/d3d-api/app/models"
	"github.com/smetroid/d3d-api/app/services"
)

type MenuController struct {
	Echo            *echo.Echo
	MenuService     services.MenuService
	AuthMiddleware  echo.MiddlewareFunc
	LogMenuRequests bool
}

func (dc *MenuController) Init() {
	dc.Echo.POST("/menu", dc.createMenu, dc.AuthMiddleware)
	dc.Echo.POST("/menu/:menu/update", dc.updateMenu, dc.AuthMiddleware)
	dc.Echo.GET("/menus", dc.getMenus, dc.AuthMiddleware)
	dc.Echo.GET("/menu/:menu", dc.getMenu, dc.AuthMiddleware)
	dc.Echo.GET("/menus_options", dc.getMenusOptions, dc.AuthMiddleware)
	dc.Echo.DELETE("/menu/:menu", dc.deleteMenu, dc.AuthMiddleware)
}

func (mc *MenuController) createMenu(ctx echo.Context) error {
	if mc.LogMenuRequests {
		// request, _ := ioutil.ReadAll(ctx.Request().Body)
		// log.Println(string(request))
		// log.Println(string("test"))
	}

	var newMenu models.Menu

	err := ctx.Bind(&newMenu)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse(err.Error()))
	}

	menusResponse, err := mc.MenuService.ProcessMenu(newMenu)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}

	return ctx.JSON(http.StatusCreated, menusResponse)
}

func (dc *MenuController) getMenus(ctx echo.Context) error {
	ctx.QueryParams()
	menusResponse, err := dc.MenuService.GetMenus(ctx.QueryParams())
	return dc.StandardResponse(ctx, menusResponse, err)

}

func (dc *MenuController) getMenu(ctx echo.Context) error {
	ctx.QueryParams()
	menuResponse, err := dc.MenuService.GetMenu(ctx.Param("menu"))
	return dc.StandardResponse(ctx, menuResponse, err)
}

func (dc *MenuController) getMenusOptions(ctx echo.Context) error {
	ctx.QueryParams()
	menusOptionsResponse, err := dc.MenuService.GetMenusOptions(ctx.QueryParams())
	return dc.StandardResponse(ctx, menusOptionsResponse, err)
}

func (dc *MenuController) deleteMenu(ctx echo.Context) error {
	err := dc.MenuService.DeleteMenu(ctx.Param("menu"))
	return dc.StandardResponse(ctx, struct {
		Status string `json:"status"`
	}{Status: "ok"}, err)
}

func (ac *MenuController) StandardResponse(ctx echo.Context, response interface{}, err error) error {
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	return ctx.JSON(http.StatusOK, response)
}

func (mc *MenuController) updateMenu(ctx echo.Context) error {
	var menuUpdate models.Menu

	//	log.Println("menuUpdate")
	//	log.Println(menuUpdate)
	//	log.Println("ctx")
	//	log.Println(ctx)
	//	request, _ := ioutil.ReadAll(ctx.Request().Body)
	//	log.Println(string(request))
	err := ctx.Bind(&menuUpdate)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse(err.Error()))
	}

	err = mc.MenuService.UpdateMenu(ctx.Param("menu"), menuUpdate)
	return mc.StandardResponse(ctx, models.OK_RESPONSE, err)
}

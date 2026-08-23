package controllers

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo"
	"github.com/smetroid/d3d-api/app/db/postgres"
	"github.com/smetroid/d3d-api/app/models"
)

type GroupsController struct {
	Echo           *echo.Echo
	DB             *postgres.Postgres
	AuthMiddleware echo.MiddlewareFunc
}

func (gc *GroupsController) Init() {
	gc.Echo.POST("/company/:id/groups", gc.createGroup, gc.AuthMiddleware)
	gc.Echo.GET("/company/:id/groups", gc.listGroups, gc.AuthMiddleware)
	gc.Echo.GET("/groups/:id", gc.getGroup, gc.AuthMiddleware)
	gc.Echo.PUT("/groups/:id/members", gc.addGroupMember, gc.AuthMiddleware)
	gc.Echo.DELETE("/groups/:id/members/:userId", gc.removeGroupMember, gc.AuthMiddleware)
	gc.Echo.DELETE("/groups/:id", gc.deleteGroup, gc.AuthMiddleware)
}

func (gc *GroupsController) createGroup(ctx echo.Context) error {
	companyId := ctx.Param("id")
	caller := usernameFromCtx(ctx)

	ok, err := gc.DB.IsMember(caller, companyId)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	if !ok {
		return ctx.JSON(http.StatusForbidden, models.ErrorResponse("not a company member"))
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse(err.Error()))
	}
	if req.Name == "" {
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse("name is required"))
	}

	g := models.Group{
		Id:        uuid.New().String(),
		Name:      req.Name,
		CompanyId: companyId,
		CreatedAt: time.Now(),
	}
	id, err := gc.DB.CreateGroup(g)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	g.Id = id
	return ctx.JSON(http.StatusCreated, map[string]interface{}{"status": "ok", "group": g})
}

func (gc *GroupsController) listGroups(ctx echo.Context) error {
	companyId := ctx.Param("id")
	caller := usernameFromCtx(ctx)

	ok, err := gc.DB.IsMember(caller, companyId)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	if !ok {
		return ctx.JSON(http.StatusForbidden, models.ErrorResponse("not a company member"))
	}

	groups, err := gc.DB.ListGroupsByCompany(companyId)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	return ctx.JSON(http.StatusOK, map[string]interface{}{"status": "ok", "groups": groups})
}

func (gc *GroupsController) getGroup(ctx echo.Context) error {
	id := ctx.Param("id")
	caller := usernameFromCtx(ctx)

	g, err := gc.DB.GetGroup(id)
	if err == postgres.ErrNotFound {
		return ctx.JSON(http.StatusNotFound, models.ErrorResponse("group not found"))
	}
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}

	ok, err := gc.DB.IsMember(caller, g.CompanyId)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	if !ok {
		return ctx.JSON(http.StatusForbidden, models.ErrorResponse("not a company member"))
	}

	members, err := gc.DB.GetGroupMembers(id)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	return ctx.JSON(http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"group":   g,
		"members": members,
	})
}

func (gc *GroupsController) addGroupMember(ctx echo.Context) error {
	id := ctx.Param("id")
	caller := usernameFromCtx(ctx)

	g, err := gc.DB.GetGroup(id)
	if err == postgres.ErrNotFound {
		return ctx.JSON(http.StatusNotFound, models.ErrorResponse("group not found"))
	}
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}

	if err := gc.requireGroupOrCompanyOwner(ctx, caller, g); err != nil {
		return err
	}

	var req struct {
		Username string `json:"username"`
	}
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse(err.Error()))
	}
	if req.Username == "" {
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse("username is required"))
	}

	// New member must already belong to the company.
	ok, err := gc.DB.IsMember(req.Username, g.CompanyId)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	if !ok {
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse("user is not a member of the company"))
	}

	if err := gc.DB.AddGroupMember(models.GroupMember{GroupId: id, UserId: req.Username}); err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	return ctx.JSON(http.StatusOK, models.OK_RESPONSE)
}

func (gc *GroupsController) removeGroupMember(ctx echo.Context) error {
	id := ctx.Param("id")
	userId := ctx.Param("userId")
	caller := usernameFromCtx(ctx)

	g, err := gc.DB.GetGroup(id)
	if err == postgres.ErrNotFound {
		return ctx.JSON(http.StatusNotFound, models.ErrorResponse("group not found"))
	}
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}

	if err := gc.requireGroupOrCompanyOwner(ctx, caller, g); err != nil {
		return err
	}

	if err := gc.DB.RemoveGroupMember(id, userId); err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	return ctx.JSON(http.StatusOK, models.OK_RESPONSE)
}

func (gc *GroupsController) deleteGroup(ctx echo.Context) error {
	id := ctx.Param("id")
	caller := usernameFromCtx(ctx)

	g, err := gc.DB.GetGroup(id)
	if err == postgres.ErrNotFound {
		return ctx.JSON(http.StatusNotFound, models.ErrorResponse("group not found"))
	}
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}

	if err := gc.requireGroupOrCompanyOwner(ctx, caller, g); err != nil {
		return err
	}

	if err := gc.DB.DeleteGroup(id); err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	return ctx.JSON(http.StatusOK, models.OK_RESPONSE)
}

// requireGroupOrCompanyOwner returns a 403 JSON response (and the error) if
// the caller is neither the company owner nor the group creator.
func (gc *GroupsController) requireGroupOrCompanyOwner(ctx echo.Context, caller string, g models.Group) error {
	company, err := gc.DB.GetCompany(g.CompanyId)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	if caller != company.CreatedBy {
		return ctx.JSON(http.StatusForbidden, models.ErrorResponse("only the group or company owner can perform this action"))
	}
	return nil
}

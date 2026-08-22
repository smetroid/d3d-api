package controllers

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo"
	"github.com/smetroid/d3d-api/app/db/postgres"
	"github.com/smetroid/d3d-api/app/models"
)

type CompaniesController struct {
	Echo           *echo.Echo
	DB             *postgres.Postgres
	AuthMiddleware echo.MiddlewareFunc
}

func (cc *CompaniesController) Init() {
	cc.Echo.POST("/company", cc.createCompany, cc.AuthMiddleware)
	cc.Echo.GET("/companies", cc.listCompanies, cc.AuthMiddleware)
	cc.Echo.GET("/company/:id", cc.getCompany, cc.AuthMiddleware)
	cc.Echo.PUT("/company/:id/members", cc.addMember, cc.AuthMiddleware)
	cc.Echo.DELETE("/company/:id/members/:userId", cc.removeMember, cc.AuthMiddleware)
	cc.Echo.DELETE("/company/:id", cc.deleteCompany, cc.AuthMiddleware)
}

func (cc *CompaniesController) createCompany(ctx echo.Context) error {
	var req struct {
		Name string `json:"name"`
	}
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse(err.Error()))
	}
	if req.Name == "" {
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse("name is required"))
	}

	caller := usernameFromCtx(ctx)
	c := models.Company{
		Id:        uuid.New().String(),
		Name:      req.Name,
		CreatedBy: caller,
		CreatedAt: time.Now(),
	}
	id, err := cc.DB.CreateCompany(c)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	c.Id = id
	return ctx.JSON(http.StatusCreated, map[string]interface{}{"status": "ok", "company": c})
}

func (cc *CompaniesController) listCompanies(ctx echo.Context) error {
	caller := usernameFromCtx(ctx)
	companies, err := cc.DB.ListCompaniesForUser(caller)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	return ctx.JSON(http.StatusOK, map[string]interface{}{"status": "ok", "companies": companies})
}

func (cc *CompaniesController) getCompany(ctx echo.Context) error {
	id := ctx.Param("id")
	caller := usernameFromCtx(ctx)

	c, err := cc.DB.GetCompany(id)
	if err == postgres.ErrNotFound {
		return ctx.JSON(http.StatusNotFound, models.ErrorResponse("company not found"))
	}
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}

	ok, err := cc.DB.IsMember(caller, id)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	if !ok {
		return ctx.JSON(http.StatusForbidden, models.ErrorResponse("not a member"))
	}

	members, err := cc.DB.GetCompanyMembers(id)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	return ctx.JSON(http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"company": c,
		"members": members,
	})
}

func (cc *CompaniesController) addMember(ctx echo.Context) error {
	id := ctx.Param("id")
	caller := usernameFromCtx(ctx)

	c, err := cc.DB.GetCompany(id)
	if err == postgres.ErrNotFound {
		return ctx.JSON(http.StatusNotFound, models.ErrorResponse("company not found"))
	}
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	if c.CreatedBy != caller {
		return ctx.JSON(http.StatusForbidden, models.ErrorResponse("only the owner can add members"))
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

	if err := cc.DB.AddMembership(models.Membership{UserId: req.Username, CompanyId: id}); err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	return ctx.JSON(http.StatusOK, models.OK_RESPONSE)
}

func (cc *CompaniesController) removeMember(ctx echo.Context) error {
	id := ctx.Param("id")
	userId := ctx.Param("userId")
	caller := usernameFromCtx(ctx)

	c, err := cc.DB.GetCompany(id)
	if err == postgres.ErrNotFound {
		return ctx.JSON(http.StatusNotFound, models.ErrorResponse("company not found"))
	}
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	if c.CreatedBy != caller {
		return ctx.JSON(http.StatusForbidden, models.ErrorResponse("only the owner can remove members"))
	}
	if userId == caller {
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse("owner cannot remove themselves"))
	}

	if err := cc.DB.RemoveMembership(userId, id); err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	return ctx.JSON(http.StatusOK, models.OK_RESPONSE)
}

func (cc *CompaniesController) deleteCompany(ctx echo.Context) error {
	id := ctx.Param("id")
	caller := usernameFromCtx(ctx)

	c, err := cc.DB.GetCompany(id)
	if err == postgres.ErrNotFound {
		return ctx.JSON(http.StatusNotFound, models.ErrorResponse("company not found"))
	}
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	if c.CreatedBy != caller {
		return ctx.JSON(http.StatusForbidden, models.ErrorResponse("only the owner can delete a company"))
	}

	if err := cc.DB.DeleteCompany(id); err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	return ctx.JSON(http.StatusOK, models.OK_RESPONSE)
}

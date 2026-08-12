package controllers

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo"
	"github.com/smetroid/d3d-api/app/auth/token"
	"github.com/smetroid/d3d-api/app/db/rethinkdb"
	"github.com/smetroid/d3d-api/app/models"
	jwt "github.com/dgrijalva/jwt-go"
)

type SharesController struct {
	Echo           *echo.Echo
	DB             *rethinkdb.RethinkDB
	SigningKey      string
	AuthMiddleware echo.MiddlewareFunc
}

func (sc *SharesController) Init() {
	sc.Echo.POST("/dag/:dag/shares", sc.createShare, sc.AuthMiddleware)
	sc.Echo.POST("/dag/:dag/shares/:jti/revoke", sc.revokeShare, sc.AuthMiddleware)
}

func (sc *SharesController) createShare(ctx echo.Context) error {
	dagId := ctx.Param("dag")

	var req models.CreateShareRequest
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse(err.Error()))
	}

	if req.Role != "view" && req.Role != "edit" {
		req.Role = "view"
	}
	if req.ExpDays <= 0 {
		req.ExpDays = 7
	}

	jti := uuid.New().String()
	exp := time.Now().Add(time.Duration(req.ExpDays) * 24 * time.Hour)

	claims := jwt.MapClaims{
		"jti":    jti,
		"iss":    "d3d-share",
		"dag_id": dagId,
		"role":   req.Role,
		"exp":    exp.Unix(),
	}
	tok, err := token.CreateToken(sc.SigningKey, claims)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}

	share := models.Share{
		Id:        uuid.New().String(),
		DagId:     dagId,
		Jti:       jti,
		Role:      req.Role,
		CreatedBy: usernameFromCtx(ctx),
		ExpiresAt: exp,
		CreatedAt: time.Now(),
	}
	if err := sc.DB.CreateShare(share); err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}

	return ctx.JSON(http.StatusCreated, models.CreateShareResponse{
		Status: "ok",
		Token:  tok,
		Jti:    jti,
		Role:   req.Role,
	})
}

func (sc *SharesController) revokeShare(ctx echo.Context) error {
	jti := ctx.Param("jti")
	if err := sc.DB.RevokeShare(jti); err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	return ctx.JSON(http.StatusOK, models.OK_RESPONSE)
}

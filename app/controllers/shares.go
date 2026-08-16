package controllers

import (
	"fmt"
	"math/rand"
	"net/http"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/google/uuid"
	"github.com/labstack/echo"
	"github.com/smetroid/d3d-api/app/auth/token"
	"github.com/smetroid/d3d-api/app/db/postgres"
	"github.com/smetroid/d3d-api/app/models"
)

var anonAdjectives = []string{
	"Amber", "Blue", "Cyan", "Emerald", "Fuchsia",
	"Gold", "Indigo", "Jade", "Lime", "Magenta",
	"Navy", "Olive", "Pink", "Rose", "Ruby",
	"Sage", "Silver", "Teal", "Violet", "Yellow",
}

var anonAnimals = []string{
	"Bear", "Crow", "Deer", "Eagle", "Fox",
	"Hawk", "Hare", "Ibis", "Jay", "Kite",
	"Lynx", "Moth", "Newt", "Owl", "Pike",
	"Quail", "Raven", "Seal", "Toad", "Vole",
}

func randomAnonName() string {
	return anonAdjectives[rand.Intn(len(anonAdjectives))] + " " + anonAnimals[rand.Intn(len(anonAnimals))]
}

type SharesController struct {
	Echo           *echo.Echo
	DB             *postgres.Postgres
	SigningKey     string
	AuthMiddleware echo.MiddlewareFunc
}

func (sc *SharesController) Init() {
	sc.Echo.GET("/shares/exchange", sc.exchangeShare) // no auth — public endpoint
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
		AnonName:  randomAnonName(),
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

func (sc *SharesController) exchangeShare(ctx echo.Context) error {
	raw := ctx.QueryParam("token")
	if raw == "" {
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse("missing token"))
	}

	tok, err := jwt.Parse(raw, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(sc.SigningKey), nil
	})
	if err != nil || !tok.Valid {
		return ctx.JSON(http.StatusUnauthorized, models.ErrorResponse("invalid or expired token"))
	}

	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return ctx.JSON(http.StatusUnauthorized, models.ErrorResponse("invalid claims"))
	}

	iss, _ := claims["iss"].(string)
	if iss != "d3d-share" {
		return ctx.JSON(http.StatusUnauthorized, models.ErrorResponse("not a share token"))
	}

	jti, _ := claims["jti"].(string)
	dagId, _ := claims["dag_id"].(string)
	role, _ := claims["role"].(string)

	revoked, err := sc.DB.IsRevoked(jti)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}
	if revoked {
		return ctx.JSON(http.StatusForbidden, models.ErrorResponse("share link revoked"))
	}

	share, err := sc.DB.GetShareByJti(jti)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse(err.Error()))
	}

	return ctx.JSON(http.StatusOK, models.ExchangeShareResponse{
		Status:   "ok",
		DagId:    dagId,
		Role:     role,
		Jti:      jti,
		AnonName: share.AnonName,
	})
}

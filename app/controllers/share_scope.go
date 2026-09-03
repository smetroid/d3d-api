package controllers

import (
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo"
	"github.com/smetroid/d3d-api/app/db/postgres"
	"github.com/smetroid/d3d-api/app/models"
)

// ShareResourceBinding returns middleware implementing layer 2 of the
// share-token scoping design (layer 1 is route scoping, done by wiring
// share-accessible routes with middleware.ShareJWTWithConfig instead of the
// general AuthMiddleware — see that function's doc comment).
//
// Layer 1 alone is decorative: it only limits which routes a share token
// can reach, not which diagram. GET /dag/:dag/history and friends all take
// a free-form :dag route parameter, so without this check a share token
// minted for one diagram could read or edit any other diagram by id — e.g.
// by combining it with GET /dags (which lists every diagram id).
//
// On any route carrying a :dag parameter, this middleware looks up the
// share by its jti (Postgres.GetShareByJti) and requires the share's bound
// dag_id to equal the requested :dag. It rejects with 403 otherwise. It
// also enforces the revocation denylist (layer 3) for every share token,
// including on the parameter-less routes, since a revoked link must not
// reach anything.
// Ordinary session tokens are not share tokens (shareInfoFromCtx returns
// isShare=false for them) and pass through unconstrained — this middleware
// only scopes share tokens, per the deliberately-deferred decision not to
// add per-user ownership scoping for session tokens.
//
// Routes with no :dag parameter (currently only GET /menus) are unaffected:
// ctx.Param("dag") is empty, so the binding check is skipped and the
// request proceeds — layer 1 already decided such a route is share-
// accessible; layer 2 has nothing to bind on it.
//
// This middleware must run after a middleware that has already parsed the
// JWT and populated ctx.Get("user") (i.e. middleware.ShareJWTWithConfig),
// and must only be attached to routes deliberately made share-accessible;
// it performs no route scoping of its own.
func ShareResourceBinding(db *postgres.Postgres) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			jti, _, isShare := shareInfoFromCtx(ctx)
			if !isShare {
				return next(ctx)
			}

			// Layer 3: revocation. RevokeShare only writes to
			// share_denylist — the shares row survives, so the binding
			// check below would happily pass a revoked link. This is the
			// only chokepoint every share-accessible route passes through,
			// so the denylist is enforced here rather than per-handler:
			// before this check lived here, only dagWS consulted it and a
			// revoked link kept working on GET /dag/:dag, /history and
			// POST /dag/:dag/update until the JWT's own exp, up to ExpDays
			// (7) later.
			revoked, err := db.IsRevoked(jti)
			if err != nil {
				log.Printf("share scope: revocation check failed for jti %s: %v", jti, err)
				return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse("could not verify share"))
			}
			if revoked {
				return ctx.JSON(http.StatusForbidden, models.ErrorResponse("share link revoked"))
			}

			dagParam := ctx.Param("dag")
			if dagParam == "" {
				return next(ctx)
			}

			share, err := db.GetShareByJti(jti)
			if err != nil {
				// A missing share is an authorization failure; a database
				// that is unreachable is not. Collapsing both into 403
				// made a Postgres restart look to recipients (and to
				// anyone reading logs) like a permanent denial.
				if errors.Is(err, postgres.ErrNotFound) {
					return ctx.JSON(http.StatusForbidden, models.ErrorResponse("invalid share"))
				}
				log.Printf("share scope: share lookup failed for jti %s: %v", jti, err)
				return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse("could not verify share"))
			}
			if share.DagId != dagParam {
				return ctx.JSON(http.StatusForbidden, models.ErrorResponse("share token not valid for this diagram"))
			}

			return next(ctx)
		}
	}
}

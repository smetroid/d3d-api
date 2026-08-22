package app

import (
	"io"
	"log"

	"github.com/labstack/echo"
	"github.com/smetroid/d3d-api/app/auth"
	"github.com/smetroid/d3d-api/app/auth/localauth"
	"github.com/smetroid/d3d-api/app/auth/middleware"
	"github.com/smetroid/d3d-api/app/collab"
	"github.com/smetroid/d3d-api/app/config"
	"github.com/smetroid/d3d-api/app/controllers"
	"github.com/smetroid/d3d-api/app/services"
	"github.com/unrolled/render"
)

type RenderWrapper struct { // We need to wrap the renderer because we need a different signature for echo.
	rnd *render.Render
}

func (r *RenderWrapper) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return r.rnd.HTML(w, 0, name, data) // The zero status code is overwritten by echo.
}

func BuildApp(config config.SamusConfig) (e *echo.Echo) {
	err := config.Postgres.Init()
	if err != nil {
		log.Fatal(err)
	}
	db := &config.Postgres

	//loadConfiguration()
	r := &RenderWrapper{render.New(render.Options{
		Layout: "layout/base",
		//Funcs:  []template.FuncMap{map[string]interface{}{"navigation": navigation}},
	},
	)}
	// Echo instance

	e = echo.New()
	e.Renderer = r
	//e.SetRenderer(r)

	// Middleware
	//e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		//AllowOrigins: []string{"http://192.168.1.4:3000", "http://192.168.1.4:8081"},
		AllowOrigins: []string{"*"},
		//AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept},
		AllowHeaders: []string{"*"},
	}))

	authProvider := BuildAuthProvider(config)
	authMiddleware := middleware.JWTWithConfig(middleware.JWTConfig{
		SigningKey:  []byte(config.Samus.SigningKey),
		TokenLookup: "header:Authorization,query:api-key,query:token",
	})

	authController := controllers.AuthController{
		Echo:         e,
		AuthProvider: authProvider,
	}
	authController.Init()

	dagService := services.DAGService{
		DB: db,
	}

	edgeService := services.EdgeService{
		DB: db,
	}

	nodeService := services.NodeService{
		DB: db,
	}

	menuService := services.MenuService{
		DB: db,
	}

	hub := collab.NewHub()

	dagController := controllers.DAGsController{
		Echo:           e,
		DAGService:     dagService,
		Hub:            hub,
		DB:             db,
		AuthMiddleware: authMiddleware,
		LogDAGRequests: config.Samus.LogDAGRequests,
	}

	edgeController := controllers.EdgeController{
		Echo:            e,
		EdgeService:     edgeService,
		AuthMiddleware:  authMiddleware,
		LogEdgeRequests: config.Samus.LogEdgeRequests,
	}

	nodeController := controllers.NodeController{
		Echo:            e,
		NodeService:     nodeService,
		AuthMiddleware:  authMiddleware,
		LogNodeRequests: config.Samus.LogNodeRequests,
	}

	menuController := controllers.MenuController{
		Echo:            e,
		MenuService:     menuService,
		AuthMiddleware:  authMiddleware,
		LogMenuRequests: config.Samus.LogMenuRequests,
	}

	sharesController := controllers.SharesController{
		Echo:           e,
		DB:             db,
		SigningKey:     config.Samus.SigningKey,
		AuthMiddleware: authMiddleware,
	}

	companiesController := controllers.CompaniesController{
		Echo:           e,
		DB:             db,
		AuthMiddleware: authMiddleware,
	}

	groupsController := controllers.GroupsController{
		Echo:           e,
		DB:             db,
		AuthMiddleware: authMiddleware,
	}

	dagController.Init()
	edgeController.Init()
	nodeController.Init()
	menuController.Init()
	sharesController.Init()
	companiesController.Init()
	groupsController.Init()

	// Route => handler
	/*
		e.GET("/samus", samus)
		e.GET("/navigation", navigation)
		e.GET("/index", index)
		e.GET("/logo", logo)
		e.GET("/yaml", yaml)
	*/

	e.Static("/css", "css")
	e.Static("/js", "js")
	e.Static("/public", "public")
	e.Static("/fonts", "fonts")
	e.Static("/vue", "vue")
	//e.Static("/node_modules", "node_modules")
	//e.GET("/dagrelib", dagrelib)
	e.GET("/samus", samus)
	//e.GET("/samus2", samus)
	e.GET("/yaml", yaml)
	e.GET("/test", test)
	//e.GET("/", index)
	e.Static("/static", "vue/dist/static")
	e.Static("/static2", "static")
	e.GET("/", func(ctx echo.Context) error {
		return ctx.Redirect(301, "/static/index.html")
	})
	return
}

func BuildAuthProvider(config config.SamusConfig) (authProvider auth.AuthProvider) {
	switch config.Samus.AuthProvider {
	case "ldap":
		authProvider = &config.Ldap
	case "oauth":
		authProvider = &config.OAuth
	case "local", "localauth":
		authProvider = &localauth.LocalAuthProvider{DB: &config.Postgres}
	}

	err := authProvider.Connect()
	defer authProvider.Close()

	if err != nil {
		log.Fatal(err)
	}
	if config.Samus.SigningKey == "" {
		log.Fatal("Shutting down, signing key must be provided.")
	}
	authProvider.SetSigningKey(config.Samus.SigningKey)
	return
}

func samus(c echo.Context) error {
	return c.File("vue/dist/index.html")
}

func test(c echo.Context) error {
	return c.File("public/test.html")
}

func yaml(c echo.Context) error {
	return c.File("public/yaml.html")
}

package main

import (
	"log"

	"github.com/smetroid/d3d-api/app"
	"github.com/smetroid/d3d-api/app/config"
)

const version = "Samus 0.1.0"
const usage = `Samus
Usage:
	d3d-api server [--config=<config>]
	d3d-api createAgentToken <name> [--config=<config>]
	d3d-api --help
	d3d-api --version
Options:
  --config=<config>            d3d-api config [default: ./d3d-api.toml].
  --help                       Show this screen.
  --version                    Show version.
`

func main() {
	//args, err := docopt.Parse(usage, nil, true, version, false)
	//if err != nil {
	//	log.Fatalln(err)
	//}
	//configFile := args["--config"].(string)
	configFile := "./d3d-api.toml"
	config := config.BuildConfig(configFile)

	//if args["server"].(bool) {
	echo := app.BuildApp(config)
	//echo.Use(middleware.Recover())
	log.Println("Starting samus server...")
	//e := echo.New()
	//echo.Use(middleware.Recover())
	//e.Logger().Fatal(e.Run(fasthttp.New(":3001")))
	echo.Start(config.Samus.BindAddr)

	//var err error

	/*
		if config.Samus.TLSEnabled {
			if config.Samus.TLSAutoEnabled {
				err = echo.StartAutoTLS(config.Samus.BindAddr)
			} else {
				err = echo.StartTLS(config.Samus.BindAddr, config.Samus.TLSCert, config.Samus.TLSKey)
			}
		} else {
			err = echo.Start(config.Samus.BindAddr)
		}

		if err != nil {
			echo.Logger.Fatal(err)
		}
	*/

	//}

	/*
		if args["createAgentToken"].(bool) {
			fmt.Println(token.CreateExpirationFreeAgentToken(args["<name>"].(string), config.Samus.SigningKey))
		}
	*/
}

package main

import (
	"log"

	"github.com/smetroid/d3d-api/app"
	"github.com/smetroid/d3d-api/app/config"
)

const version = "Samus 0.1.0"
const usage = `Samus
Usage:
	samus server [--config=<config>]
	samus createAgentToken <name> [--config=<config>]
	samus --help
	samus --version
Options:
  --config=<config>            Samus config [default: ./samus.toml].
  --help                       Show this screen.
  --version                    Show version.
`

func main() {
	//args, err := docopt.Parse(usage, nil, true, version, false)
	//if err != nil {
	//	log.Fatalln(err)
	//}
	//configFile := args["--config"].(string)
	configFile := "./samus.toml"
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

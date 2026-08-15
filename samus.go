package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/smetroid/d3d-api/app"
	"github.com/smetroid/d3d-api/app/config"
	"github.com/smetroid/d3d-api/app/models"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	configFile := "./samus.toml"
	if len(os.Args) >= 2 && os.Args[1] == "--config" && len(os.Args) >= 3 {
		configFile = os.Args[2]
	}

	// createUser <username> <password> [--config=<file>]
	if len(os.Args) >= 2 && os.Args[1] == "createUser" {
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "usage: samus createUser <username> <password>")
			os.Exit(1)
		}
		username := os.Args[2]
		password := os.Args[3]
		if len(os.Args) >= 6 && os.Args[4] == "--config" {
			configFile = os.Args[5]
		}

		cfg := config.BuildConfig(configFile)
		if err := cfg.Postgres.Init(); err != nil {
			log.Fatal("postgres init:", err)
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			log.Fatal("bcrypt:", err)
		}
		user := models.User{
			Id:           uuid.New().String(),
			Username:     username,
			PasswordHash: string(hash),
			CreatedAt:    time.Now(),
		}
		if err := cfg.Postgres.CreateUser(user); err != nil {
			log.Fatal("create user:", err)
		}
		fmt.Printf("user %q created\n", username)
		return
	}

	cfg := config.BuildConfig(configFile)
	echo := app.BuildApp(cfg)
	log.Println("Starting samus server...")
	echo.Start(cfg.Samus.BindAddr)
}

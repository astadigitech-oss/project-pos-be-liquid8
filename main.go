package main

import (
	"liquid8/pos/api"
	"liquid8/pos/config"
	"log"
	"os"

	// "liquid8/pos/models"
	"github.com/gin-gonic/gin"
)

func main() {
	config.InitDB()
	appEnv := os.Getenv("APP_ENV")
	var server *gin.Engine

	if appEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
		server = gin.New()
		server.Use(gin.Recovery())
		log.Println("--- PRODUCTION MODE ---")
	} else {
		gin.SetMode(gin.DebugMode)
		server = gin.Default()
		log.Println("--- DEVELOPMENT MODE ---")
	}

	// ============================
	// STATIC FILE CONFIG
	// ============================
	server.Static("/public", "./public")

	// Middleware CORS
	server.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*") // frontend origin
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Header("Access-Control-Allow-Credentials", "true") // optional jika kamu pakai cookies

		// Kalau OPTIONS, langsung balas OK tanpa lanjut handler lain
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	api.RouteHandler(server)

	server.Run(":5002")
}
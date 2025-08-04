package main

import (
	"log"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func main() {

	// Initialize MongoDB connection
	if err := InitMongoDB(); err != nil {
		log.Fatalf("Failed to initialize MongoDB: %v", err)
	}
	defer CloseMongoDB()

	// Set Gin mode
	ginMode := gin.DebugMode
	gin.SetMode(ginMode)

	// Create Gin router
	router := gin.Default()
	trustedProxies := os.Getenv("TRUSTED_PROXIES")
	if trustedProxies == "" {
		// Default to common private network CIDRs
		trustedProxies = "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16"
	}
	router.SetTrustedProxies(strings.Split(trustedProxies, ","))

	// 2. Add Proxy-Aware Middleware
	router.Use(func(c *gin.Context) {
		// Ensure X-Forwarded-* headers are respected
		if clientIP := c.ClientIP(); clientIP != "" {
			c.Request.RemoteAddr = clientIP
		}
		c.Next()
	})

	// Add middleware
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// Register routes
	RegisterRoutes(router)

	// Get port from environment or use default
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting server on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

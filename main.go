package main

import (
	"log"
	"os"

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

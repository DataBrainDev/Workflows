package main

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

var triggerData map[string]interface{}

// RegisterRoutes registers all API routes
func RegisterRoutes(router *gin.Engine) {
	// Health check endpoint
	router.GET("/api/v1/health", HealthCheck)

	// Workflow management endpoints
	router.POST("/api/v1/save/:workflowID", SaveWorkflow)
	router.GET("/api/v1/get/:workflowID", GetWorkflow)
	router.DELETE("/api/v1/delete/:workflowID", DeleteWorkflow)
	router.GET("/api/v1/get_all", GetAllWorkflows)
	router.POST("/api/v1/run/:workflowID", RunWorkflow)
}

// HealthCheck returns the health status of the API
func HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"message": "Workflow Engine API is running",
	})
}

// SaveWorkflow saves or updates a workflow in MongoDB
func SaveWorkflow(c *gin.Context) {
	workflowID := c.Param("workflowID")
	if workflowID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "workflowID parameter is required",
		})
		return
	}

	var workflowData map[string]interface{}
	if err := c.ShouldBindJSON(&workflowData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid JSON format: " + err.Error(),
		})
		return
	}

	// Validate that the JSON contains a workflow structure
	if _, ok := workflowData["workflow"]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "JSON must contain a 'workflow' field",
		})
		return
	}

	// Save to MongoDB
	err := SaveWorkflowToDB(workflowID, workflowData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to save workflow: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Workflow saved successfully",
		"workflowID": workflowID,
	})
}

// GetWorkflow retrieves a workflow from MongoDB
func GetWorkflow(c *gin.Context) {
	workflowID := c.Param("workflowID")
	if workflowID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "workflowID parameter is required",
		})
		return
	}

	workflow, err := GetWorkflowFromDB(workflowID)
	if err != nil {
		if err.Error() == "workflow not found" {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Workflow not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve workflow: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, workflow)
}

// DeleteWorkflow deletes a workflow from MongoDB
func DeleteWorkflow(c *gin.Context) {
	workflowID := c.Param("workflowID")
	if workflowID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "workflowID parameter is required",
		})
		return
	}

	err := DeleteWorkflowFromDB(workflowID)
	if err != nil {
		if err.Error() == "workflow not found" {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Workflow not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to delete workflow: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Workflow deleted successfully",
		"workflowID": workflowID,
	})
}

// GetAllWorkflows returns all workflow IDs from MongoDB
func GetAllWorkflows(c *gin.Context) {
	workflowIDs, err := GetAllWorkflowIDsFromDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve workflow IDs: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"workflowIDs": workflowIDs,
		"count":       len(workflowIDs),
	})
}

// RunWorkflow retrieves and executes a workflow
func RunWorkflow(c *gin.Context) {
	workflowID := c.Param("workflowID")
	if workflowID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "workflowID parameter is required",
		})
		return
	}

	// Read the request body
	if err := c.ShouldBindJSON(&triggerData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid JSON format: " + err.Error(),
		})
		return
	}

	// Get workflow from MongoDB
	workflowData, err := GetWorkflowFromDB(workflowID)
	if err != nil {
		if err.Error() == "workflow not found" {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Workflow not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve workflow: " + err.Error(),
		})
		return
	}

	// Convert workflow data to JSON string for engine
	workflowJSON, err := json.Marshal(workflowData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to marshal workflow data: " + err.Error(),
		})
		return
	}

	// Create and execute workflow engine
	engine, err := NewWorkflowEngine(string(workflowJSON))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create workflow engine: " + err.Error(),
		})
		return
	}

	// Execute workflow
	if err := engine.Execute(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "Workflow execution failed: " + err.Error(),
			"workflowID": workflowID,
		})
		return
	}

	// Prepare execution results
	results := make(map[string]interface{})
	for nodeID, result := range engine.context.NodeResults {
		results[nodeID] = result
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Workflow executed successfully",
		"workflowID":  workflowID,
		"nodeResults": results,
	})
}

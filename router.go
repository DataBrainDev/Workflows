package main

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Global variable to store trigger data
var triggerData map[string]interface{}

// RegisterRoutes registers all API routes
func RegisterRoutes(router *gin.Engine) {
	// Health check endpoint
	router.GET("/api/v1/health", HealthCheck)

	// Workflow management endpoints
	router.POST("/api/v1/save/:workflowID", SaveWorkflow)       // Save or update a workflow
	router.GET("/api/v1/get/:workflowID", GetWorkflow)          // Retrieve a workflow
	router.DELETE("/api/v1/delete/:workflowID", DeleteWorkflow) // Delete a workflow
	router.GET("/api/v1/get_all", GetAllWorkflows)              // Get all workflow IDs
	router.POST("/api/v1/run/:workflowID", RunWorkflow)         // Execute a workflow
}

// HealthCheck returns the health status of the API
func HealthCheck(c *gin.Context) {
	// Respond with a simple JSON indicating the API is running
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"message": "Workflow Engine API is running",
	})
}

// SaveWorkflow saves or updates a workflow in MongoDB
func SaveWorkflow(c *gin.Context) {
	// Extract workflowID from the URL parameter
	workflowID := c.Param("workflowID")
	if workflowID == "" {
		// Return error if workflowID is missing
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "workflowID parameter is required",
		})
		return
	}

	// Parse the JSON body into a map
	var workflowData map[string]interface{}
	if err := c.ShouldBindJSON(&workflowData); err != nil {
		// Return error if JSON is invalid
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid JSON format: " + err.Error(),
		})
		return
	}

	// Validate that the JSON contains a "workflow" field
	if _, ok := workflowData["workflow"]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "JSON must contain a 'workflow' field",
		})
		return
	}

	// Save the workflow to MongoDB
	err := SaveWorkflowToDB(workflowID, workflowData)
	if err != nil {
		// Return error if saving to the database fails
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to save workflow: " + err.Error(),
		})
		return
	}

	// Respond with success message
	c.JSON(http.StatusOK, gin.H{
		"message":    "Workflow saved successfully",
		"workflowID": workflowID,
	})
}

// GetWorkflow retrieves a workflow from MongoDB
func GetWorkflow(c *gin.Context) {
	// Extract workflowID from the URL parameter
	workflowID := c.Param("workflowID")
	if workflowID == "" {
		// Return error if workflowID is missing
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "workflowID parameter is required",
		})
		return
	}

	// Retrieve the workflow from MongoDB
	workflow, err := GetWorkflowFromDB(workflowID)
	if err != nil {
		// Handle "workflow not found" error
		if err.Error() == "workflow not found" {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Workflow not found",
			})
			return
		}
		// Handle other errors
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve workflow: " + err.Error(),
		})
		return
	}

	// Respond with the retrieved workflow
	c.JSON(http.StatusOK, workflow)
}

// DeleteWorkflow deletes a workflow from MongoDB
func DeleteWorkflow(c *gin.Context) {
	// Extract workflowID from the URL parameter
	workflowID := c.Param("workflowID")
	if workflowID == "" {
		// Return error if workflowID is missing
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "workflowID parameter is required",
		})
		return
	}

	// Delete the workflow from MongoDB
	err := DeleteWorkflowFromDB(workflowID)
	if err != nil {
		// Handle "workflow not found" error
		if err.Error() == "workflow not found" {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Workflow not found",
			})
			return
		}
		// Handle other errors
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to delete workflow: " + err.Error(),
		})
		return
	}

	// Respond with success message
	c.JSON(http.StatusOK, gin.H{
		"message":    "Workflow deleted successfully",
		"workflowID": workflowID,
	})
}

// GetAllWorkflows returns all workflow IDs from MongoDB
func GetAllWorkflows(c *gin.Context) {
	// Retrieve all workflow IDs from MongoDB
	workflowIDs, err := GetAllWorkflowIDsFromDB()
	if err != nil {
		// Return error if retrieval fails
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve workflow IDs: " + err.Error(),
		})
		return
	}

	// Respond with the list of workflow IDs and their count
	c.JSON(http.StatusOK, gin.H{
		"workflowIDs": workflowIDs,
		"count":       len(workflowIDs),
	})
}

// RunWorkflow retrieves and executes a workflow
func RunWorkflow(c *gin.Context) {
	// Extract workflowID from the URL parameter
	workflowID := c.Param("workflowID")
	if workflowID == "" {
		// Return error if workflowID is missing
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "workflowID parameter is required",
		})
		return
	}

	// Parse the JSON body into the global triggerData variable
	if err := c.ShouldBindJSON(&triggerData); err != nil {
		// Return error if JSON is invalid
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid JSON format: " + err.Error(),
		})
		return
	}

	// Retrieve the workflow from MongoDB
	workflowData, err := GetWorkflowFromDB(workflowID)
	if err != nil {
		// Handle "workflow not found" error
		if err.Error() == "workflow not found" {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Workflow not found",
			})
			return
		}
		// Handle other errors
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve workflow: " + err.Error(),
		})
		return
	}

	// Convert the workflow data to a JSON string for the workflow engine
	workflowJSON, err := json.Marshal(workflowData)
	if err != nil {
		// Return error if marshalling fails
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to marshal workflow data: " + err.Error(),
		})
		return
	}

	// Create a new workflow engine instance
	engine, err := NewWorkflowEngine(string(workflowJSON))
	if err != nil {
		// Return error if engine creation fails
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create workflow engine: " + err.Error(),
		})
		return
	}

	// Execute the workflow
	if err := engine.Execute(); err != nil {
		// Return error if execution fails
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

	// Respond with execution results
	c.JSON(http.StatusOK, gin.H{
		"message":     "Workflow executed successfully",
		"workflowID":  workflowID,
		"nodeResults": results,
	})
}

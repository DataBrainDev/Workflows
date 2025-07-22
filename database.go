package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	mongoClient *mongo.Client
	database    *mongo.Database
	collection  *mongo.Collection
)

// WorkflowDocument represents the structure of a workflow document in MongoDB
type WorkflowDocument struct {
	ID           string                 `bson:"_id"`
	WorkflowID   string                 `bson:"workflowID"`
	WorkflowData map[string]interface{} `bson:"workflowData"`
	CreatedAt    time.Time              `bson:"createdAt"`
	UpdatedAt    time.Time              `bson:"updatedAt"`
}

// InitMongoDB initializes the MongoDB connection
func InitMongoDB() error {
	// Get MongoDB connection string from environment
	mongoURI := "mongodb://localhost:27017"

	// Get database name from environment
	dbName := "workflow_engine"

	// Get collection name from environment
	collectionName := "workflows"

	// Set client options
	clientOptions := options.Client().ApplyURI(mongoURI)

	// Connect to MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Ping the database to verify connection
	err = client.Ping(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	// Set global variables
	mongoClient = client
	database = client.Database(dbName)
	collection = database.Collection(collectionName)

	// Create indexes
	if err := createIndexes(); err != nil {
		log.Printf("Warning: Failed to create indexes: %v", err)
	}

	log.Println("Successfully connected to MongoDB")
	return nil
}

// createIndexes creates necessary indexes for the collection
func createIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create index on workflowID for faster queries
	indexModel := mongo.IndexModel{
		Keys:    bson.D{{Key: "workflowID", Value: 1}},
		Options: options.Index().SetUnique(true),
	}

	_, err := collection.Indexes().CreateOne(ctx, indexModel)
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}

	return nil
}

// CloseMongoDB closes the MongoDB connection
func CloseMongoDB() {
	if mongoClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := mongoClient.Disconnect(ctx); err != nil {
			log.Printf("Error disconnecting from MongoDB: %v", err)
		} else {
			log.Println("Disconnected from MongoDB")
		}
	}
}

// SaveWorkflowToDB saves or updates a workflow in MongoDB
func SaveWorkflowToDB(workflowID string, workflowData map[string]interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()

	// No need to create a separate document as the update object handles the data

	// Use upsert to create or update
	filter := bson.M{"workflowID": workflowID}
	update := bson.M{
		"$set": bson.M{
			"workflowData": workflowData,
			"updatedAt":    now,
		},
		"$setOnInsert": bson.M{
			"_id":        workflowID,
			"workflowID": workflowID,
			"createdAt":  now,
		},
	}

	opts := options.Update().SetUpsert(true)
	_, err := collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to save workflow: %w", err)
	}

	return nil
}

// GetWorkflowFromDB retrieves a workflow from MongoDB
func GetWorkflowFromDB(workflowID string) (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var doc WorkflowDocument
	filter := bson.M{"workflowID": workflowID}

	err := collection.FindOne(ctx, filter).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, errors.New("workflow not found")
		}
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}

	return doc.WorkflowData, nil
}

// DeleteWorkflowFromDB deletes a workflow from MongoDB
func DeleteWorkflowFromDB(workflowID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{"workflowID": workflowID}
	result, err := collection.DeleteOne(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to delete workflow: %w", err)
	}

	if result.DeletedCount == 0 {
		return errors.New("workflow not found")
	}

	return nil
}

// GetAllWorkflowIDsFromDB returns all workflow IDs from MongoDB
func GetAllWorkflowIDsFromDB() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Only project the workflowID field
	opts := options.Find().SetProjection(bson.M{"workflowID": 1, "_id": 0})
	cursor, err := collection.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow IDs: %w", err)
	}
	defer cursor.Close(ctx)

	var workflowIDs []string
	for cursor.Next(ctx) {
		var result struct {
			WorkflowID string `bson:"workflowID"`
		}
		if err := cursor.Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode workflow ID: %w", err)
		}
		workflowIDs = append(workflowIDs, result.WorkflowID)
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	return workflowIDs, nil
}

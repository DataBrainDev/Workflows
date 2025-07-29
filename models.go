package main

// Workflow represents the entire workflow structure, including metadata, nodes, connections, and configuration.
type Workflow struct {
	Workflow    WorkflowInfo           `json:"workflow"`    // Metadata about the workflow.
	Nodes       []Node                 `json:"nodes"`       // List of nodes in the workflow.
	Connections []Connection           `json:"connections"` // Connections between nodes.
	Config      map[string]interface{} `json:"config"`      // Additional configuration for the workflow.
}

// WorkflowInfo contains metadata about the workflow.
type WorkflowInfo struct {
	Name        string `json:"name"`        // Name of the workflow.
	Description string `json:"description"` // Description of the workflow.
}

// Node represents a single node in the workflow.
type Node struct {
	ID         string                 `json:"id"`         // Unique identifier for the node.
	Name       string                 `json:"name"`       // Name of the node.
	Type       string                 `json:"type"`       // Type of the node (e.g., task, decision).
	Parameters map[string]interface{} `json:"parameters"` // Parameters specific to the node.
	Position   int                    `json:"position"`   // Position of the node in the workflow.
	Retry      RetryConfig            `json:"retry"`      // Retry configuration for the node.
}

// RetryConfig defines the retry behavior for a node.
type RetryConfig struct {
	Enabled     bool `json:"enabled"`     // Whether retries are enabled.
	MaxAttempts int  `json:"maxAttempts"` // Maximum number of retry attempts.
	Delay       int  `json:"delay"`       // Delay between retries in seconds.
}

// Connection represents a link between two nodes in the workflow.
type Connection struct {
	From   string `json:"from"`             // ID of the source node.
	To     string `json:"to"`               // ID of the destination node.
	Branch string `json:"branch,omitempty"` // Optional branch name for conditional connections.
}

// Condition represents a single condition used in decision-making.
type Condition struct {
	Value1    string      `json:"value1"`    // First value in the condition.
	Operation string      `json:"operation"` // Operation to compare the values (e.g., ==, >, <).
	Value2    interface{} `json:"value2"`    // Second value in the condition.
}

// NumberCondition groups multiple numeric conditions.
type NumberCondition struct {
	Number []Condition `json:"number"` // List of numeric conditions.
}

// ExecutionContext holds the runtime state of the workflow execution.
type ExecutionContext struct {
	NodeResults map[string]map[string]interface{} // Results of each node execution.
	Config      map[string]interface{}            // Runtime configuration.
}

// WorkflowEngine is responsible for executing the workflow.
type WorkflowEngine struct {
	workflow *Workflow         // The workflow to be executed.
	context  *ExecutionContext // The execution context for the workflow.
}

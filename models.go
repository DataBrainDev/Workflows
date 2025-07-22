package main

type Workflow struct {
	Workflow    WorkflowInfo           `json:"workflow"`
	Nodes       []Node                 `json:"nodes"`
	Connections []Connection           `json:"connections"`
	Config      map[string]interface{} `json:"config"`
}

type WorkflowInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Node struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Type       string                 `json:"type"`
	Parameters map[string]interface{} `json:"parameters"`
	Position   int                    `json:"position"`
	Retry      RetryConfig            `json:"retry"`
}

type RetryConfig struct {
	Enabled     bool `json:"enabled"`
	MaxAttempts int  `json:"maxAttempts"`
	Delay       int  `json:"delay"`
}

type Connection struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Branch string `json:"branch,omitempty"`
}

type Condition struct {
	Value1    string      `json:"value1"`
	Operation string      `json:"operation"`
	Value2    interface{} `json:"value2"`
}

type NumberCondition struct {
	Number []Condition `json:"number"`
}

type ExecutionContext struct {
	NodeResults map[string]map[string]interface{}
	Config      map[string]interface{}
}

type WorkflowEngine struct {
	workflow *Workflow
	context  *ExecutionContext
}

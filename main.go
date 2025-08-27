package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync" 
	"time"
)

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

var (
    lastOrderPayload map[string]interface{} // Stores the latest posted JSON
    lastOrderMutex   = &sync.Mutex{}
)

func NewWorkflowEngine(workflowJSON string) (*WorkflowEngine, error) {
	var workflow Workflow
	if err := json.Unmarshal([]byte(workflowJSON), &workflow); err != nil {
		return nil, fmt.Errorf("failed to parse workflow JSON: %w", err)
	}

	return &WorkflowEngine{
		workflow: &workflow,
		context: &ExecutionContext{
			NodeResults: make(map[string]map[string]interface{}),
			Config:      workflow.Config,
		},
	}, nil
}

func (we *WorkflowEngine) Execute() error {
	log.Printf("Starting workflow: %s", we.workflow.Workflow.Name)

	startNode := we.getStartNode()
	if startNode == nil {
		return fmt.Errorf("no starting node found")
	}

	currentNode := startNode
	for currentNode != nil {
		log.Printf("Executing node: %s (%s)", currentNode.Name, currentNode.ID)

		if err := we.executeNode(currentNode); err != nil {
			return fmt.Errorf("failed to execute node %s: %w", currentNode.ID, err)
		}

		nextNodeID := we.getNextNodeID(currentNode)
		if nextNodeID == "" {
			break // End of workflow
		}

		currentNode = we.getNodeByID(nextNodeID)
		if currentNode == nil {
			return fmt.Errorf("node not found: %s", nextNodeID)
		}
	}

	log.Printf("Workflow completed successfully: %s", we.workflow.Workflow.Name)
	return nil
}

func (we *WorkflowEngine) getNextNodeID(currentNode *Node) string {
	connections := we.getConnectionsFrom(currentNode.ID)
	if len(connections) == 0 {
		return "" // No outgoing connections
	}

	// Handle condition nodes
	if currentNode.Type == "if" {
		resultData := we.context.NodeResults[currentNode.ID]
		conditionResult := resultData["conditionResult"].(bool)

		for _, conn := range connections {
			if (conditionResult && conn.Branch == "true") ||
				(!conditionResult && conn.Branch == "false") {
				return conn.To
			}
		}
	}

	// Default: first connection
	return connections[0].To
}

func (we *WorkflowEngine) getConnectionsFrom(nodeID string) []Connection {
	var conns []Connection
	for _, conn := range we.workflow.Connections {
		if conn.From == nodeID {
			conns = append(conns, conn)
		}
	}
	return conns
}

func (we *WorkflowEngine) getStartNode() *Node {
	for i := range we.workflow.Nodes {
		if we.workflow.Nodes[i].Position == 1 {
			return &we.workflow.Nodes[i]
		}
	}
	return nil
}

func (we *WorkflowEngine) executeNode(node *Node) error {
	var err error
	maxAttempts := 1
	delay := time.Duration(0)

	if node.Retry.Enabled {
		maxAttempts = node.Retry.MaxAttempts
		delay = time.Duration(node.Retry.Delay) * time.Millisecond
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		log.Printf("Executing %s (attempt %d/%d)", node.Name, attempt, maxAttempts)

		switch node.Type {
		case "httpRequest":
			err = we.executeHTTPRequest(node)

		case "sqlQuery":
			err = we.executeSQLQuery(node)
		case "if":
			err = we.executeIfCondition(node)
		case "arrayMap":
			err = we.executeArrayMap(node)
		default:
			err = fmt.Errorf("unsupported node type: %s", node.Type)
		}

		if err == nil {
			log.Printf("Node %s executed successfully", node.Name)
			return nil
		}

		if attempt < maxAttempts {
			log.Printf("Node %s failed (attempt %d/%d): %v. Retrying in %v...",
				node.Name, attempt, maxAttempts, err, delay)
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("node %s failed after %d attempts: %w", node.Name, maxAttempts, err)
}

func (we *WorkflowEngine) applyTemplateFunctions(value interface{}, funcCalls []string) (interface{}, error) {
	var currentValue = value

	for _, call := range funcCalls {
		// Trim spaces from the entire call string first
		call = strings.TrimSpace(call)
		// Use SplitN to handle arguments containing colons
		parts := strings.SplitN(call, ":", 2)

		// Trim spaces from function name
		funcName := strings.TrimSpace(parts[0])
		var args []string

		if len(parts) > 1 {
			// Split arguments and trim each one
			for _, arg := range strings.Split(parts[1], ",") {
				args = append(args, strings.TrimSpace(arg))
			}
		}

		fmt.Println("[DEBUG] Function call parts:", parts)
		fmt.Println("[DEBUG] Applying function:", funcName, "with args:", args)

		switch funcName {
		case "countryToAlpha3":
			fmt.Println("[DEBUG] currentVal", "->", currentValue, "function:", funcName)
			if strVal, ok := currentValue.(string); ok {
				currentValue = countryToAlpha3(strVal)
			}
		case "truncate":
			fmt.Println("[DEBUG] currentVal", "->", currentValue, "function:", funcName)
			if len(args) != 1 {
				return nil, fmt.Errorf("truncate requires maxLen argument")
			}
			maxLen, err := strconv.Atoi(args[0])
			if err != nil {
				return nil, fmt.Errorf("invalid maxLen for truncate: %w", err)
			}
			if strVal, ok := currentValue.(string); ok {
				currentValue = truncate(strVal, maxLen)
				fmt.Println("[DEBUG] strVal", strVal, "->", currentValue)
			}
		case "join":
			if len(args) != 1 {
				return nil, fmt.Errorf("join requires one argument")
			}
			argVal := we.resolveTemplateValue(args[0])
			if str1, ok := currentValue.(string); ok {
				if str2, ok := argVal.(string); ok {
					currentValue = join(str1, str2)
				}
			}
		case "toNumber":
			num, err := toNumber(currentValue)
			if err != nil {
				return nil, fmt.Errorf("toNumber conversion failed: %w", err)
			}
			currentValue = num

		case "toBoolean":
			boolVal, err := toBoolean(currentValue)
			if err != nil {
				return nil, fmt.Errorf("toBoolean conversion failed: %w", err)
			}
			currentValue = boolVal

			fmt.Printf("[DEBUG] After applying %s, currentValue=%v (type %T)\n", funcName, currentValue, currentValue)

		case "toDate":
			d, err := toDate(currentValue)
			if err != nil {
				return nil, fmt.Errorf("toDate conversion failed: %w", err)
			}
			currentValue = d

		case "defaultIfEmpty":
			// assume args[0] is default value string
			defVal := ""
			if len(args) > 0 {
				defVal = args[0]
			}
			s, err := defaultIfEmpty(currentValue, defVal)
			if err != nil {
				return nil, fmt.Errorf("defaultIfEmpty failed: %w", err)
			}
			currentValue = s

		case "substring":
			if len(args) != 2 {
				return nil, fmt.Errorf("substring requires 2 arguments")
			}
			start, _ := strconv.Atoi(args[0])
			end, _ := strconv.Atoi(args[1])
			if strVal, ok := currentValue.(string); ok {
				currentValue = substring(strVal, start, end)
			}

		case "concat":
			allArgs := append([]string{fmt.Sprint(currentValue)}, args...)
			currentValue = concat(allArgs...)

		case "toUpperCase":
			if strVal, ok := currentValue.(string); ok {
				currentValue = toUpperCase(strVal)
			}

		case "toLowerCase":
			if strVal, ok := currentValue.(string); ok {
				currentValue = toLowerCase(strVal)
			}

		case "trim":
			if strVal, ok := currentValue.(string); ok {
				currentValue = trim(strVal)
			}

		case "split":
			if len(args) != 1 {
				return nil, fmt.Errorf("split requires 1 argument")
			}
			if strVal, ok := currentValue.(string); ok {
				currentValue = split(strVal, args[0])
			}

		case "replace":
			if len(args) != 2 {
				return nil, fmt.Errorf("replace requires 2 arguments")
			}
			if strVal, ok := currentValue.(string); ok {
				currentValue = replace(strVal, args[0], args[1])
			}

		case "strLength":
			if strVal, ok := currentValue.(string); ok {
				currentValue = strLength(strVal)
			}

		case "indexOf":
			if len(args) != 1 {
				return nil, fmt.Errorf("indexOf requires 1 argument")
			}
			if strVal, ok := currentValue.(string); ok {
				currentValue = indexOf(strVal, args[0])
			}

		case "includes":
			if len(args) != 1 {
				return nil, fmt.Errorf("includes requires 1 argument")
			}
			if strVal, ok := currentValue.(string); ok {
				currentValue = includes(strVal, args[0])
			}

		case "startsWith":
			if len(args) != 1 {
				return nil, fmt.Errorf("startsWith requires 1 argument")
			}
			if strVal, ok := currentValue.(string); ok {
				currentValue = startsWith(strVal, args[0])
			}

		case "endsWith":
			if len(args) != 1 {
				return nil, fmt.Errorf("endsWith requires 1 argument")
			}
			if strVal, ok := currentValue.(string); ok {
				currentValue = endsWith(strVal, args[0])
			}

		case "parseJSON":
			if strVal, ok := currentValue.(string); ok {
				parsed, err := parseJSON(strVal)
				if err != nil {
					return nil, err
				}
				currentValue = parsed
			}

		case "stringifyJSON":
			str, err := stringifyJSON(currentValue)
			if err != nil {
				return nil, err
			}
			currentValue = str

		case "hasOwnProperty":
			if len(args) != 1 {
				return nil, fmt.Errorf("hasOwnProperty requires 1 argument")
			}
			if m, ok := currentValue.(map[string]interface{}); ok {
				currentValue = hasOwnProperty(m, args[0])
			}

		case "keys":
			if m, ok := currentValue.(map[string]interface{}); ok {
				currentValue = keys(m)
			}

		case "values":
			if m, ok := currentValue.(map[string]interface{}); ok {
				currentValue = values(m)
			}

		case "parseInt":
			if strVal, ok := currentValue.(string); ok {
				num, err := parseInt(strVal)
				if err != nil {
					return nil, err
				}
				currentValue = num
			}

		case "parseFloat":
			if strVal, ok := currentValue.(string); ok {
				num, err := parseFloat(strVal)
				if err != nil {
					return nil, err
				}
				currentValue = num
			}

		case "toFixed":
			if len(args) != 1 {
				return nil, fmt.Errorf("toFixed requires 1 argument")
			}
			if num, ok := currentValue.(float64); ok {
				digits, _ := strconv.Atoi(args[0])
				currentValue = toFixed(num, digits)
			}
		default:
			return nil, fmt.Errorf("unknown function: %s", funcName)
		}
	}

	return currentValue, nil
}

func (we *WorkflowEngine) executeHTTPRequest(node *Node) error {
	resolvedParams := we.resolveTemplateValue(node.Parameters).(map[string]interface{})
	inputUrl := resolvedParams["url"].(string)
	method := resolvedParams["method"].(string)

	var body io.Reader
	if bodyData, ok := resolvedParams["body"]; ok {
		bodyType := "json" // default type
		if bt, ok := resolvedParams["bodyType"].(string); ok {
			bodyType = bt
		}

		headers := map[string]interface{}{}
		if h, ok := resolvedParams["headers"].(map[string]interface{}); ok {
			headers = h
		}

		// --- Begin fix ---
		// Agar bodyData string hai aur valid JSON, to unmarshal
		if str, ok := bodyData.(string); ok {
			var jsonData interface{}
			if err := json.Unmarshal([]byte(str), &jsonData); err == nil {
				bodyData = jsonData
			}
		}
		// --- End fix ---

		b, err := buildRequestBody(bodyType, bodyData, headers)
		if err != nil {
			return fmt.Errorf("failed to build request body: %w", err)
		}

		if bodyType == "json" {
			switch v := bodyData.(type) {
			case map[string]interface{}, []interface{}:
				if pretty, err := json.MarshalIndent(v, "", "  "); err == nil {
					fmt.Println("📤 Outbound JSON body:\n", string(pretty))
				}
			case string:
				// Try to parse if it's JSON string
				var parsed interface{}
				if err := json.Unmarshal([]byte(v), &parsed); err == nil {
					if pretty, err := json.MarshalIndent(parsed, "", "  "); err == nil {
						fmt.Println("📤 Outbound JSON body:\n", string(pretty))
					}
				} else {
					fmt.Println("📤 Outbound JSON body (raw string):\n", v)
				}
			default:
				fmt.Println("📤 Outbound JSON body (unknown type):", v)
			}
		}
		body = b
	}

	req, err := http.NewRequest(method, inputUrl, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if headers, ok := resolvedParams["headers"].(map[string]interface{}); ok {
		for key, value := range headers {
			req.Header.Set(key, fmt.Sprintf("%v", value))
		}
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	result := map[string]interface{}{
		"httpStatusCode": resp.StatusCode,
		"headers":        resp.Header,
		"body":           string(respBody),
	}

	var jsonResp interface{}
	if err := json.Unmarshal(respBody, &jsonResp); err == nil {
		result["json"] = jsonResp
		if jsonMap, ok := jsonResp.(map[string]interface{}); ok {
			if valueArr, ok := jsonMap["value"].([]interface{}); ok {
				result["rowCount"] = len(valueArr)
				if len(valueArr) > 0 {
					if firstItem, ok := valueArr[0].(map[string]interface{}); ok {
						for k, v := range firstItem {
							result[k] = v
						}
					}
				}
			} else {
				for k, v := range jsonMap {
					if !strings.HasPrefix(k, "@odata.") {
						result[k] = v
					}
				}
			}
		}
	}

	we.context.NodeResults[node.ID] = result

	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Helper function to build request body based on type and headers
func buildRequestBody(bodyType string, bodyData interface{}, headers map[string]interface{}) (io.Reader, error) {
	// Infer bodyType from headers if default "json"
	if bodyType == "json" {
		if ct, ok := headers["Content-Type"]; ok {
			ctStr := strings.ToLower(fmt.Sprintf("%v", ct))
			if strings.Contains(ctStr, "application/x-www-form-urlencoded") {
				bodyType = "form"
			}
		}
	}

	if bodyData == nil {
		return nil, nil
	}

	var bodyMap map[string]interface{}

	switch v := bodyData.(type) {
	case map[string]interface{}:
		bodyMap = v

	case []interface{}:
		// Agar array hai to directly marshal kardo aur return kar do
		bodyJSON, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal JSON array: %w", err)
		}
		return bytes.NewBuffer(bodyJSON), nil

	case map[string]string:
		bodyMap = make(map[string]interface{}, len(v))
		for key, val := range v {
			bodyMap[key] = val
		}

	case string:
		return strings.NewReader(v), nil
	case []byte:
		return bytes.NewReader(v), nil
	default:
		return nil, fmt.Errorf("unsupported body data type: %T", bodyData)
	}

	switch strings.ToLower(bodyType) {
	case "form", "x-www-form-urlencoded":
		form := url.Values{}
		for k, v := range bodyMap {
			form.Set(k, fmt.Sprintf("%v", v))
		}
		return strings.NewReader(form.Encode()), nil

	case "json":
		bodyJSON, err := json.Marshal(bodyMap)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal JSON: %w", err)
		}
		return bytes.NewBuffer(bodyJSON), nil

	default:
		return nil, fmt.Errorf("unsupported body type: %s", bodyType)
	}
}

// func (we *WorkflowEngine) executeTriggerNode(node *Node) error {
// 	// Create the result structure to store in node results
// 	result := map[string]interface{}{
// 		"triggerData": triggerData,
// 	}

// 	// Store the result in the workflow context
// 	we.context.NodeResults[node.ID] = result
// 	return nil
// }

func (we *WorkflowEngine) executeArrayMap(node *Node) error {
	resolvedParams, ok := we.resolveTemplateValue(node.Parameters).(map[string]interface{})
	if !ok {
		return fmt.Errorf("failed to resolve parameters for node %s", node.ID)
	}

	sourcePath, ok := resolvedParams["sourceArray"].(string)
	if !ok || sourcePath == "" {
		return fmt.Errorf("sourceArray path not provided")
	}

	fmt.Println("🔹 [ArrayMap] Node:", node.Name, "(", node.ID, ")")
	fmt.Println("✅ Resolved Parameters:", resolvedParams)
	fmt.Println("📌 Source Path:", sourcePath)

	// ✅ Resolve the source array
	sourceArray := we.resolveExpression(sourcePath)
	fmt.Printf("📌 Resolved SourceArray Value: %#v\n", sourceArray)
	fmt.Printf("📌 Resolved SourceArray Type: %T\n", sourceArray)

	var items []interface{}

	switch v := sourceArray.(type) {
	case []interface{}:
		items = v
	case string:
		var parsed []interface{}
		err := json.Unmarshal([]byte(v), &parsed)
		if err != nil {
			return fmt.Errorf("executeArrayMap: sourceArray is string but not JSON array, value=%v", v)
		}
		items = parsed
	case map[string]interface{}:
		for _, obj := range v {
			items = append(items, obj)
		}
	default:
		return fmt.Errorf("executeArrayMap: sourceArray is not an array, type=%T, value=%#v", sourceArray, sourceArray)
	}

	itemMapping, ok := resolvedParams["itemMapping"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("itemMapping not provided or invalid")
	}

	mappedArray := []map[string]interface{}{}
	for _, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		mappedItem := map[string]interface{}{}

		for key, template := range itemMapping {
			resolvedValue := we.resolveTemplateWithContext(template, itemMap)
			mappedItem[key] = resolvedValue
		}
		mappedArray = append(mappedArray, mappedItem)
	}

	// ✅ Convert mappedArray into JSON
	jsonOutput, err := json.MarshalIndent(mappedArray, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal mapped array: %w", err)
	}

	we.context.NodeResults[node.ID] = map[string]interface{}{
		"output": string(jsonOutput),
	}

	fmt.Println("✅ Final Mapped Array:", string(jsonOutput))
	return nil
}

func (we *WorkflowEngine) mapObject(item map[string]interface{}, mapping map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	for targetField, sourceTemplate := range mapping {
		// Convert interface{} to string
		sourceStr, ok := sourceTemplate.(string)
		if !ok {
			result[targetField] = sourceTemplate
			continue
		}

		// Check for {{$json["field"]}} pattern
		if strings.HasPrefix(sourceStr, "{{$json[") && strings.HasSuffix(sourceStr, "]}}") {
			// Extract the key inside $json["key"]
			key := strings.TrimPrefix(sourceStr, "{{$json[")
			key = strings.TrimSuffix(key, "]}}")
			key = strings.Trim(key, "\"' ") // remove quotes if any

			if val, exists := item[key]; exists {
				result[targetField] = val
			} else {
				result[targetField] = nil
			}
		} else {
			// fallback: normal path resolution
			result[targetField] = we.getNestedValue(item, sourceStr)
		}
	}

	return result
}

func (we *WorkflowEngine) executeSQLQuery(node *Node) error {
	resolvedParams := we.resolveTemplateValue(node.Parameters).(map[string]interface{})
	query := resolvedParams["query"].(string)
	connectionString := resolvedParams["connectionString"].(string)

	db, err := sql.Open("sqlserver", connectionString)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	var results []map[string]interface{}
	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("failed to get columns: %w", err)
	}

	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return fmt.Errorf("failed to scan row: %w", err)
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			row[strings.ToLower(col)] = values[i]
		}
		results = append(results, row)
	}

	result := map[string]interface{}{
		"results":  results,
		"rowCount": len(results),
	}

	we.context.NodeResults[node.ID] = result
	return nil
}

func (we *WorkflowEngine) executeIfCondition(node *Node) error {
	we.context.NodeResults[node.ID] = map[string]interface{}{
		"conditionResult": we.evaluateCondition(node),
	}
	return nil
}

func (we *WorkflowEngine) evaluateCondition(node *Node) bool {
	conditions, ok := node.Parameters["conditions"].(map[string]interface{})
	if !ok {
		return false
	}

	numberConds, ok := conditions["number"].([]interface{})
	if !ok {
		return false
	}

	for _, condInterface := range numberConds {
		condMap, ok := condInterface.(map[string]interface{})
		if !ok {
			continue
		}

		value1Str := we.resolveTemplateValue(condMap["value1"]).(string)
		operation := condMap["operation"].(string)
		value2 := condMap["value2"]

		val1, err1 := strconv.ParseFloat(value1Str, 64)
		val2, err2 := strconv.ParseFloat(fmt.Sprintf("%v", value2), 64)

		if err1 != nil || err2 != nil {
			continue
		}

		switch operation {
		case "equals":
			if val1 == val2 {
				return true
			}
		case "greater":
			if val1 > val2 {
				return true
			}
		case "less":
			if val1 < val2 {
				return true
			}
		}
	}

	return false
}
func (we *WorkflowEngine) resolveStringOrValue(s string) interface{} {
	trimmed := strings.TrimSpace(s)

	// If the whole string is exactly {{ ... }}, return the raw value directly
	reWhole := regexp.MustCompile(`^\{\{(.*)\}\}$`)
	if m := reWhole.FindStringSubmatch(trimmed); len(m) == 2 {
		inner := strings.TrimSpace(m[1])
		return we.resolveExpression(inner)
	}

	// Otherwise, resolve {{...}} placeholders inside the string and return a string
	return we.resolveStringTemplates(s)
}

func (we *WorkflowEngine) resolveTemplateValue(value interface{}) interface{} {
	switch v := value.(type) {
	case string:
		return we.resolveStringOrValue(v)
	case map[string]interface{}:
		resolved := make(map[string]interface{})
		for k, val := range v {
			resolved[k] = we.resolveTemplateValue(val)
		}
		return resolved
	case []interface{}:
		resolved := make([]interface{}, len(v))
		for i, val := range v {
			resolved[i] = we.resolveTemplateValue(val)
		}
		return resolved
	default:
		return v
	}
}

func (we *WorkflowEngine) resolveTemplateWithContext(template interface{}, context map[string]interface{}) interface{} {
	str, ok := template.(string)
	if !ok {
		return template
	}

	// Check if template exactly matches a key in context
	if val, exists := context[str]; exists {
		return val
	}

	// Replace {{key}} placeholders
	for k, v := range context {
		placeholder := fmt.Sprintf("{{%s}}", k)
		str = strings.ReplaceAll(str, placeholder, fmt.Sprintf("%v", v))
	}

	return str
}

func (we *WorkflowEngine) resolveStringTemplates(template string) string {
	re := regexp.MustCompile(`\{\{(.*?)\}\}`)
	return re.ReplaceAllStringFunc(template, func(match string) string {
		inner := strings.Trim(match, "{}")
		return fmt.Sprintf("%v", we.resolveExpression(inner))
	})
}

// func (we *WorkflowEngine) resolveExpression(expr string) interface{} {
// 	parts := strings.Split(expr, "|")
// 	baseExpr := strings.TrimSpace(parts[0])
// 	funcCalls := parts[1:]

// 	value := we.resolveBaseExpression(baseExpr)
// 	if value == nil {
// 		return ""
// 	}

// 	if len(funcCalls) > 0 {
// 		result, err := we.applyTemplateFunctions(value, funcCalls)
// 		if err != nil {
// 			log.Printf("Error applying functions: %v", err)
// 			return ""
// 		}
// 		return result
// 	}

// 	return value
// }

func (we *WorkflowEngine) resolveExpression(expr string) interface{} {
	trim := strings.TrimSpace(expr)
	if strings.HasPrefix(trim, "$") {
		reFn := regexp.MustCompile(`^\$(\w+)\((.*)\)$`)
		if m := reFn.FindStringSubmatch(trim); len(m) == 3 {
			fn := strings.ToLower(m[1])
			argsRaw := strings.TrimSpace(m[2])

			// split argsRaw into up to 3 top-level args: arg1, arg2[, arg3]
			arg1, arg2, arg3 := "", "", ""
			depth := 0
			last := 0
			parts := []string{}
			for i, ch := range argsRaw {
				switch ch {
				case '{', '[':
					depth++
				case '}', ']':
					if depth > 0 {
						depth--
					}
				case ',':
					if depth == 0 {
						parts = append(parts, strings.TrimSpace(argsRaw[last:i]))
						last = i + 1
					}
				}
				_ = i
			}
			parts = append(parts, strings.TrimSpace(argsRaw[last:]))

			if len(parts) > 0 {
				arg1 = parts[0]
			}
			if len(parts) > 1 {
				arg2 = parts[1]
			}
			if len(parts) > 2 {
				arg3 = parts[2]
			}

			switch fn {
			case "jsonencode":
				inner := we.resolveExpression(arg1)
				s, err := stringifyJSON(inner)
				if err != nil {
					log.Printf("jsonEncode failed: %v", err)
					return ""
				}
				return s

			case "parsejson":
				inner := we.resolveExpression(arg1)
				if str, ok := inner.(string); ok {
					v, err := parseJSON(str)
					if err != nil {
						log.Printf("parseJSON failed: %v", err)
						return nil
					}
					return v
				}
				return inner

			case "arraymap":
				// arg1: source array expr
				source := we.resolveExpression(arg1)

				// arg2: mapping literal or expression returning a map
				var mapping map[string]interface{}
				if strings.HasPrefix(arg2, "{") {
					var any interface{}
					if err := json.Unmarshal([]byte(arg2), &any); err != nil {
						log.Printf("arrayMap mapping parse failed: %v", err)
						return nil
					}
					mapping, _ = any.(map[string]interface{})
				} else {
					if mv, ok := we.resolveExpression(arg2).(map[string]interface{}); ok {
						mapping = mv
					}
				}
				if mapping == nil {
					log.Printf("arrayMap: mapping is nil or invalid")
					return nil
				}

				coerce := strings.EqualFold(strings.Trim(arg3, `"' `), "json")
				out, err := we.arrayMapEval(source, mapping, coerce)
				if err != nil {
					log.Printf("arrayMap failed: %v", err)
					return nil
				}

				return out

			case "coalesce":
				// Return the first non-empty / non-nil value among up to 3 args
				for _, raw := range []string{arg1, arg2, arg3} {
					if strings.TrimSpace(raw) == "" {
						continue
					}
					v := we.resolveExpression(raw)
					switch vv := v.(type) {
					case nil:
						continue
					case string:
						if strings.TrimSpace(vv) != "" {
							return vv
						}
					default:
						// any non-nil non-string is accepted
						return v
					}
				}
				return ""
			}
		}
	}

	parts := strings.Split(expr, "|")
	baseExpr := strings.TrimSpace(parts[0])
	funcCalls := parts[1:]

	value := we.resolveBaseExpression(baseExpr)
	if value == nil {
		return ""
	}

	if len(funcCalls) > 0 {
		result, err := we.applyTemplateFunctions(value, funcCalls)
		if err != nil {
			log.Printf("Error applying functions: %v", err)
			return ""
		}
		return result
	}
	return value
}

func (we *WorkflowEngine) arrayMapEval(
	source interface{},
	mapping map[string]interface{},
	coerceNumbers bool,
) ([]map[string]interface{}, error) {
	var items []interface{}

	switch v := source.(type) {
	case []interface{}:
		items = v
	case string:
		var parsed []interface{}
		if err := json.Unmarshal([]byte(v), &parsed); err != nil {
			return nil, fmt.Errorf("arrayMap: source string is not JSON array: %w", err)
		}
		items = parsed
	case map[string]interface{}:
		for _, obj := range v {
			items = append(items, obj)
		}
	default:
		return nil, fmt.Errorf("arrayMap: source is not array-like, got %T", source)
	}

	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		row, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		mapped := make(map[string]interface{}, len(mapping))
		for k, tmpl := range mapping {
			val := we.resolveTemplateWithContext(tmpl, row)
			if coerceNumbers {
				if s, ok := val.(string); ok {
					if i, err := strconv.Atoi(s); err == nil {
						val = i
					} else if f, err := strconv.ParseFloat(s, 64); err == nil {
						val = f
					}
				}
			}
			mapped[k] = val
		}
		out = append(out, mapped)
	}
	pretty, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println("DocumentLines Generated:", string(pretty))

	return out, nil

}

func (we *WorkflowEngine) resolveBaseExpression(expr string) interface{} {
	if strings.HasPrefix(expr, "config.") {
		key := strings.TrimPrefix(expr, "config.")
		return we.context.Config[key]
	}

	if strings.HasPrefix(expr, "$node['") {
		re := regexp.MustCompile(`\$node\['([^']+)'\]\.(.+)`)
		matches := re.FindStringSubmatch(expr)
		if len(matches) != 3 {
			return expr
		}

		nodeID := matches[1]
		fieldPath := matches[2]

		nodeResult, ok := we.context.NodeResults[nodeID]
		if !ok {
			return expr
		}

		return we.getNestedValue(nodeResult, fieldPath)
	}

	return expr
}

func (we *WorkflowEngine) getNestedValue(data interface{}, path string) interface{} {
	parts := strings.Split(path, ".")
	current := data

	for _, part := range parts {
		if current == nil {
			return nil
		}

		if strings.Contains(part, "[") && strings.Contains(part, "]") {
			re := regexp.MustCompile(`^([^\[]+)\[['"]?([^]'"]+)['"]?\]$`)
			matches := re.FindStringSubmatch(part)
			if len(matches) != 3 {
				return part
			}

			fieldName := matches[1]
			key := matches[2]

			currentMap, ok := current.(map[string]interface{})
			if !ok {
				return part
			}

			fieldValue, exists := currentMap[fieldName]
			if !exists {
				return part
			}

			switch v := fieldValue.(type) {
			case []interface{}:
				index, err := strconv.Atoi(key)
				if err != nil || index < 0 || index >= len(v) {
					return part
				}
				current = v[index]
			case map[string]interface{}:
				if val, exists := v[key]; exists {
					current = val
				} else {
					return part
				}
			default:
				return part
			}
		} else {
			currentMap, ok := current.(map[string]interface{})
			if !ok {
				return part
			}

			if val, exists := currentMap[part]; exists {
				current = val
			} else {
				return part
			}
		}
	}
	return current
}

func (we *WorkflowEngine) getNodeByID(id string) *Node {
	for i := range we.workflow.Nodes {
		if we.workflow.Nodes[i].ID == id {
			return &we.workflow.Nodes[i]
		}
	}
	return nil
}

// countryToAlpha3 converts a country name to its ISO Alpha-3 code.
// Returns "   " if the country is not found in the map.
func countryToAlpha3(country string) string {
	countryMap := map[string]string{
		"united states":  "USA",
		"united kingdom": "GBR",
		"germany":        "DEU",
		"france":         "FRA",
		"canada":         "CAN",
		"australia":      "AUS",
		"india":          "IND",
		"china":          "CHN",
		"japan":          "JPN",
		"brazil":         "BRA",
		"israel":         "IL",
	}

	normalized := strings.ToLower(strings.TrimSpace(country))
	if code, ok := countryMap[normalized]; ok {
		return code
	}
	return "   "
}

// truncate shortens a string to the specified maximum length.
func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}

// join concatenates two strings with a space in between.
func join(str1, str2 string) string {
	return strings.TrimSpace(str1 + " " + str2)
}

// toNumber converts various types (float64, int, string) to a float64.
// Returns an error if the type is unsupported or conversion fails.
func toNumber(value interface{}) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case string:
		if v == "" {
			return 0, nil
		}
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("unsupported type for toNumber: %T", value)
	}
}

// toBoolean converts various types (bool, string) to a boolean.
// Returns an error if the type is unsupported or conversion fails.
func toBoolean(value interface{}) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		s := strings.ToLower(strings.TrimSpace(v))
		if s == "" {
			return false, nil
		}
		if s == "true" || s == "1" || s == "yes" || s == "y" || s == "tyes" {
			return true, nil
		}
		if s == "false" || s == "0" || s == "no" || s == "n" || s == "tno" {
			return false, nil
		}
		return false, fmt.Errorf("cannot convert string to bool: %q", v)
	default:
		return false, fmt.Errorf("unsupported type for toBoolean: %T", value)
	}
}

func toDate(value interface{}) (string, error) {
	// Expects date string in formats like "2006-01-02", "2006-01-02T15:04:05Z", etc.
	// Returns date formatted as "2006-01-02" or empty string if input invalid/empty.
	switch v := value.(type) {
	case string:
		s := strings.TrimSpace(v)
		if s == "" || s == "null" {
			return "", nil
		}
		// Try parsing multiple formats:
		formats := []string{
			"2006-01-02",
			"2006-01-02T15:04:05Z07:00",
			"2006-01-02 15:04:05",
		}
		for _, f := range formats {
			if t, err := time.Parse(f, s); err == nil {
				return t.Format("2006-01-02"), nil
			}
		}
		return "", fmt.Errorf("cannot parse date: %q", v)
	default:
		return "", fmt.Errorf("unsupported type for toDate: %T", value)
	}
}

func defaultIfEmpty(value interface{}, defaultVal string) (string, error) {
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return defaultVal, nil
		}
		return v, nil
	case nil:
		return defaultVal, nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// substring extracts a portion of a string between start and end indices.
func substring(s string, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end > len(s) {
		end = len(s)
	}
	if start > end {
		start, end = end, start
	}
	if start > len(s) || end < 0 {
		return ""
	}
	return s[start:end]
}

// concat concatenates multiple strings into one.
func concat(strs ...string) string {
	return strings.Join(strs, "")
}

// toUpperCase converts a string to uppercase.
func toUpperCase(s string) string {
	return strings.ToUpper(s)
}

// toLowerCase converts a string to lowercase.
func toLowerCase(s string) string {
	return strings.ToLower(s)
}

// trim removes leading and trailing whitespace from a string.
func trim(s string) string {
	return strings.TrimSpace(s)
}

// split splits a string into a slice of substrings based on a separator.
func split(s, sep string) []string {
	return strings.Split(s, sep)
}

// replace replaces the first occurrence of a substring with a new string.
func replace(s, old, new string) string {
	return strings.Replace(s, old, new, 1)
}

// strLength returns the length of a string.
func strLength(s string) int {
	return len(s)
}

// indexOf returns the index of the first occurrence of a substring in a string.
// Returns -1 if the substring is not found.
func indexOf(s, substr string) int {
	return strings.Index(s, substr)
}

// includes checks if a substring exists within a string.
func includes(s, substr string) bool {
	return strings.Contains(s, substr)
}

// startsWith checks if a string starts with a specified prefix.
func startsWith(s, prefix string) bool {
	return strings.HasPrefix(s, prefix)
}

// endsWith checks if a string ends with a specified suffix.
func endsWith(s, suffix string) bool {
	return strings.HasSuffix(s, suffix)
}

// parseJSON parses a JSON string into a Go data structure (map, slice, etc.).
func parseJSON(s string) (interface{}, error) {
	var data interface{}
	err := json.Unmarshal([]byte(s), &data)
	return data, err
}

// stringifyJSON converts a Go data structure into a JSON string.
func stringifyJSON(v interface{}) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

// hasOwnProperty checks if a map contains a specific key.
func hasOwnProperty(obj map[string]interface{}, key string) bool {
	_, ok := obj[key]
	return ok
}

// keys returns a slice of all keys in a map.
func keys(obj map[string]interface{}) []string {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	return keys
}

// values returns a slice of all values in a map.
func values(obj map[string]interface{}) []interface{} {
	vals := make([]interface{}, 0, len(obj))
	for _, v := range obj {
		vals = append(vals, v)
	}
	return vals
}

// parseInt converts a string to an integer.
// Returns 0 if the string is empty.
func parseInt(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.Atoi(s)
}

// parseFloat converts a string to a float64.
// Returns 0 if the string is empty.
func parseFloat(s string) (float64, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.ParseFloat(s, 64)
}

// toFixed formats a float64 to a string with a specified number of decimal places.
func toFixed(num float64, digits int) string {
	return strconv.FormatFloat(num, 'f', digits, 64)
}

// POST /createOrder -> store JSON and trigger workflow
func createOrderHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
        return
    }

    body, err := io.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "Failed to read request body", http.StatusBadRequest)
        return
    }
    defer r.Body.Close()

    // Parse any JSON dynamically
    var data map[string]interface{}
    if err := json.Unmarshal(body, &data); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }

    // Store posted JSON
    lastOrderMutex.Lock()
    lastOrderPayload = data
    lastOrderMutex.Unlock()

    pretty, _ := json.MarshalIndent(data, "", "  ")
    fmt.Println("📥 Received JSON Payload:\n", string(pretty))

    // Load workflow file
    jsonBytes, err := os.ReadFile("gms-sap.json")
    if err != nil {
        http.Error(w, "Failed to read workflow file", http.StatusInternalServerError)
        return
    }

    engine, err := NewWorkflowEngine(string(jsonBytes))
    if err != nil {
        http.Error(w, "Failed to create workflow engine", http.StatusInternalServerError)
        return
    }

    // Override config to point to our local endpoint
    if engine.context.Config == nil {
        engine.context.Config = map[string]interface{}{}
    }
    engine.context.Config["gmsDummyApiUrl"] = "http://localhost:8089"

    // Execute the workflow now
    if err := engine.Execute(); err != nil {
        http.Error(w, "Workflow execution failed", http.StatusInternalServerError)
        return
    }

    // Send response
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    w.Write([]byte(`{"status":"success","message":"JSON received successfully"}`))
}

// GET /order -> used by workflow's first node to fetch posted JSON
func orderHandler(w http.ResponseWriter, r *http.Request) {
    lastOrderMutex.Lock()
    payload := lastOrderPayload
    lastOrderMutex.Unlock()

    if payload == nil {
        http.Error(w, "No order payload posted yet", http.StatusNotFound)
        return
    }

    // Workflow expects the inner object if JSON contains {"Order": {...}}
    if inner, ok := payload["Order"].(map[string]interface{}); ok {
        payload = inner
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(payload)
}


func main() {
	// http.HandleFunc("/createOrder", createOrderHandler)
    // http.HandleFunc("/order", orderHandler)

    // fmt.Println("🚀 Server running at http://localhost:8089")
    // log.Fatal(http.ListenAndServe(":8089", nil))

	jsonBytes, err := os.ReadFile("sap-gms-salsorder.json")
	if err != nil {
		log.Fatalf("Failed to read workflow file: %v", err)
	}
	workflowJSON := string(jsonBytes)

	engine, err := NewWorkflowEngine(workflowJSON)
	if err != nil {
		log.Fatalf("Failed to create workflow engine: %v", err)
	}

	if err := engine.Execute(); err != nil {
		log.Fatalf("Workflow execution failed: %v", err)
	}

	log.Println("=== Workflow Execution Results ===")
	for nodeID, result := range engine.context.NodeResults {
		log.Printf("Node %s completed successfully", nodeID)
		if httpStatus, ok := result["httpStatusCode"]; ok {
			log.Printf("  HTTP Status: %v", httpStatus)
		}
		if rowCount, ok := result["rowCount"]; ok {
			log.Printf("  Row Count: %v", rowCount)
		}

	}

}

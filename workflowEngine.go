package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
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

// func (we *WorkflowEngine) Execute() error {
// 	log.Printf("Starting workflow: %s", we.workflow.Workflow.Name)

// 	executionOrder, err := we.buildExecutionOrder()
// 	if err != nil {
// 		return fmt.Errorf("failed to build execution order: %w", err)
// 	}

// 	for _, nodeID := range executionOrder {
// 		node := we.getNodeByID(nodeID)
// 		if node == nil {
// 			return fmt.Errorf("node not found: %s", nodeID)
// 		}

// 		log.Printf("Executing node: %s (%s)", node.Name, node.ID)

// 		if err := we.executeNode(node); err != nil {
// 			return fmt.Errorf("failed to execute node %s: %w", node.ID, err)
// 		}
// 	}

// 	log.Printf("Workflow completed successfully: %s", we.workflow.Workflow.Name)
// 	return nil
// }

func (we *WorkflowEngine) buildExecutionOrder() ([]string, error) {
	var order []string
	visited := make(map[string]bool)

	var startNode *Node
	for _, node := range we.workflow.Nodes {
		if node.Position == 1 {
			startNode = &node
			break
		}
	}

	if startNode == nil {
		return nil, fmt.Errorf("no starting node found")
	}

	we.dfsOrder(startNode.ID, visited, &order)
	return order, nil
}

func (we *WorkflowEngine) dfsOrder(nodeID string, visited map[string]bool, order *[]string) {
	if visited[nodeID] {
		return
	}

	visited[nodeID] = true
	*order = append(*order, nodeID)

	for _, conn := range we.workflow.Connections {
		if conn.From == nodeID {
			if conn.Branch != "" {
				fromNode := we.getNodeByID(conn.From)
				if fromNode != nil && fromNode.Type == "if" {
					conditionResult := we.evaluateCondition(fromNode)
					shouldExecute := (conn.Branch == "true" && conditionResult) ||
						(conn.Branch == "false" && !conditionResult)
					if shouldExecute {
						we.dfsOrder(conn.To, visited, order)
					}
				}
			} else {
				we.dfsOrder(conn.To, visited, order)
			}
		}
	}
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
		case "trigger":
			err = we.executeTriggerNode(node)
		case "sqlQuery":
			err = we.executeSQLQuery(node)
		case "if":
			err = we.executeIfCondition(node)
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

		case "defaultIfEmpty":
			if len(args) != 1 {
				return nil, fmt.Errorf("defaultIfEmpty requires default value argument")
			}
			if currentValue == nil || currentValue == "" {
				currentValue = args[0]
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
		bodyJSON, err := json.Marshal(bodyData)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		body = bytes.NewBuffer(bodyJSON)
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

func (we *WorkflowEngine) executeTriggerNode(node *Node) error {
	// Create the result structure to store in node results
	result := map[string]interface{}{
		"triggerData": triggerData,
	}

	// Store the result in the workflow context
	we.context.NodeResults[node.ID] = result
	return nil
}

func (we *WorkflowEngine) executeArrayMap(node *Node) error {
	resolvedParams := we.resolveTemplateValue(node.Parameters).(map[string]interface{})
	sourceArray := we.getNestedValue(we.context.NodeResults, resolvedParams["sourceArray"].(string))

	// Handle both []interface{} (JSON arrays) and []map[string]interface{} (object arrays)
	var items []interface{}
	switch v := sourceArray.(type) {
	case []interface{}:
		items = v
	case []map[string]interface{}:
		for _, obj := range v {
			items = append(items, obj)
		}
	default:
		return fmt.Errorf("sourceArray is not an array")
	}

	// Process each object in the array
	mappedArray := []map[string]interface{}{}
	for _, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue // Skip non-object items
		}
		mappedItem := we.mapObject(itemMap, resolvedParams["itemMapping"].(map[string]interface{}))
		mappedArray = append(mappedArray, mappedItem)
	}

	we.context.NodeResults[node.ID] = map[string]interface{}{
		"output": mappedArray,
	}
	return nil
}

func (we *WorkflowEngine) mapObject(item map[string]interface{}, mapping map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for targetField, sourcePath := range mapping {
		// Resolve paths like "BillingAddress.Street"
		value := we.getNestedValue(item, sourcePath.(string))
		result[targetField] = value
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

func (we *WorkflowEngine) resolveTemplateValue(value interface{}) interface{} {
	switch v := value.(type) {
	case string:
		return we.resolveStringTemplates(v)
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

func (we *WorkflowEngine) resolveStringTemplates(template string) string {
	re := regexp.MustCompile(`\{\{(.*?)\}\}`)
	return re.ReplaceAllStringFunc(template, func(match string) string {
		inner := strings.Trim(match, "{}")
		return fmt.Sprintf("%v", we.resolveExpression(inner))
	})
}

func (we *WorkflowEngine) resolveExpression(expr string) interface{} {
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

// func (we *WorkflowEngine) getNodeByID(id string) *Node {
// 	for _, node := range we.workflow.Nodes {
// 		if node.ID == id {
// 			return &node
// 		}
// 	}
// 	return nil
// }

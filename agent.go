package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const defaultModel = "claude-3-5-sonnet-20241022"

type GenerateRequest struct {
	ChatID int
}

type GenerateResponse struct {
	ChatID int
	Error  error
}

type agent struct {
	ac *anthropic.Client
	c  *client
	gc chan GenerateRequest
}

func newAgent(ctx context.Context, c *client) (*agent, error) {
	// Get API key from environment
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable not set")
	}

	// Create anthropic client
	ac := anthropic.NewClient(option.WithAPIKey(apiKey))
	
	return &agent{
		ac: ac,
		c:  c,
		gc: make(chan GenerateRequest),
	}, nil
}

func (a *agent) getChatHistory(cid int) ([]anthropic.MessageParam, error) {
	// Get the messages in the chat
	ms, err := a.c.ListMessages(cid)
	if err != nil {
		return nil, fmt.Errorf("failed to list messages: %w", err)
	}

	// Convert them to anthropic messages
	var hs []anthropic.MessageParam
	var pendingToolUseID string
	
	for _, m := range ms {
		switch m.MType {
		case "user":
			hs = append(hs, anthropic.NewUserMessage(anthropic.NewTextBlock(m.UserMsg.Text)))
		case "agent":
			hs = append(hs, anthropic.NewAssistantMessage(anthropic.NewTextBlock(m.AgentMsg.Text)))
		case "tool":
			// For tool calls, we need to reconstruct the assistant message with tool use
			// and then add the tool result
			toolUseID := fmt.Sprintf("tool_%d", m.MessageID)
			pendingToolUseID = toolUseID
			
			// Convert tool args to JSON
			inputJSON, err := json.Marshal(m.ToolMsg.ToolArgs)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal tool args: %w", err)
			}
			
			toolUse := anthropic.ToolUseBlock{
				Type: anthropic.ContentBlockTypeToolUse,
				ID:   toolUseID,
				Name: m.ToolMsg.ToolName,
				Input: json.RawMessage(inputJSON),
			}
			hs = append(hs, anthropic.NewAssistantMessage(toolUse))
			
			// Add tool result if we have one
			if m.ToolMsg.ToolError != "" || m.ToolMsg.ToolResult != "" {
				var resultContent string
				isError := false
				
				if m.ToolMsg.ToolError != "" {
					resultContent = m.ToolMsg.ToolError
					isError = true
				} else {
					resultContent = m.ToolMsg.ToolResult
				}
				
				toolResult := anthropic.ToolResultBlock{
					Type:      anthropic.ContentBlockTypeToolResult,
					ToolUseID: pendingToolUseID,
					IsError:   isError,
					Content:   resultContent,
				}
				hs = append(hs, anthropic.NewUserMessage(toolResult))
			}
		default:
			return nil, fmt.Errorf("unknown message type %q", m.MType)
		}
	}
	return hs, nil
}

func (a *agent) generate(ctx context.Context, cid int, onupdate func()) (*Message, error) {
	// Get the previous messages from the conversation
	h, err := a.getChatHistory(cid)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	// Create the request
	req := anthropic.MessageNewParams{
		Model:     ptr(defaultModel),
		Messages:  h,
		MaxTokens: ptr(int64(4096)),
		Tools:     a.getTools(),
	}

	// Generate a response using anthropic
	resp, err := a.ac.Messages.New(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate response: %w", err)
	}

	// Process the response
	var m *Message
	
	// Check if the response contains tool use
	var hasToolUse bool
	var toolUseBlock *anthropic.ToolUseBlock
	var textContent string
	
	for _, content := range resp.Content {
		switch content.Type {
		case anthropic.ContentBlockTypeText:
			if textBlock, ok := content.AsUnion().(anthropic.TextBlock); ok {
				textContent += textBlock.Text
			}
		case anthropic.ContentBlockTypeToolUse:
			if toolBlock, ok := content.AsUnion().(anthropic.ToolUseBlock); ok {
				hasToolUse = true
				toolUseBlock = &toolBlock
			}
		}
	}

	if hasToolUse && toolUseBlock != nil {
		// Convert JSON input to map
		var toolArgs map[string]any
		if err := json.Unmarshal(toolUseBlock.Input, &toolArgs); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tool args: %w", err)
		}
		
		// Create tool message
		m = &Message{
			ChatID: cid,
			MType:  "tool",
			ToolMsg: &struct {
				ToolDone   bool
				ToolName   string
				ToolArgs   map[string]any
				ToolResult string
				ToolError  string
			}{
				ToolDone: false,
				ToolName: toolUseBlock.Name,
				ToolArgs: toolArgs,
			},
		}
	} else {
		// Create agent message
		m = &Message{
			ChatID: cid,
			MType:  "agent",
			AgentMsg: &struct{ Text string }{
				Text: textContent,
			},
		}
	}

	// Create the message in the database
	msg, err := a.c.CreateMessage(*m)
	if err != nil {
		return nil, fmt.Errorf("failed to create message: %w", err)
	}
	m = msg

	// Trigger update
	defer onupdate()

	// Handle the tool call if needed
	if m.MType == "tool" {
		fmt.Fprintln(os.Stderr, "Calling the tool")
		// NOTE: This will update the message in the client
		if err := a.handleToolCall(m); err != nil {
			return nil, fmt.Errorf("failed to handle tool call: %w", err)
		}
		m.ToolMsg.ToolDone = true
	}
	
	return m, nil
}

func (a *agent) getTools() []anthropic.ToolParam {
	return []anthropic.ToolParam{
		{
			Name:        "get_node",
			Description: ptr("Retrieves a single graph node by its ID. Returns the node's ID, type, and properties."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Type: anthropic.ToolInputSchemaTypeObject,
				Properties: map[string]interface{}{
					"id": map[string]interface{}{
						"type":        "integer",
						"description": "The unique identifier of the node to retrieve.",
					},
				},
				Required: []string{"id"},
			},
		},
		{
			Name:        "list_nodes",
			Description: ptr("Lists all graph nodes of a specific type. If no type is provided, returns all nodes."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Type: anthropic.ToolInputSchemaTypeObject,
				Properties: map[string]interface{}{
					"node_type": map[string]interface{}{
						"type":        "string",
						"description": "The type of nodes to list. If empty, all nodes will be returned.",
					},
				},
			},
		},
		{
			Name:        "create_node",
			Description: ptr("Creates a new graph node with the specified type and properties. Returns the created node with its assigned ID."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Type: anthropic.ToolInputSchemaTypeObject,
				Properties: map[string]interface{}{
					"type": map[string]interface{}{
						"type":        "string",
						"description": "The type of the node to create. For example, 'person', 'document', etc.",
					},
					"props": map[string]interface{}{
						"type":        "object",
						"description": "A map of properties to store with the node. For example, {\"name\": \"John\", \"age\": 30}.",
					},
				},
				Required: []string{"type"},
			},
		},
		{
			Name:        "delete_node",
			Description: ptr("Deletes a graph node by its ID. Note that this will also delete all edges connected to this node."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Type: anthropic.ToolInputSchemaTypeObject,
				Properties: map[string]interface{}{
					"id": map[string]interface{}{
						"type":        "integer",
						"description": "The unique identifier of the node to delete.",
					},
				},
				Required: []string{"id"},
			},
		},
		{
			Name:        "get_edge",
			Description: ptr("Retrieves a single graph edge by its ID. Returns the edge's ID, type, and the IDs of its connected nodes."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Type: anthropic.ToolInputSchemaTypeObject,
				Properties: map[string]interface{}{
					"id": map[string]interface{}{
						"type":        "integer",
						"description": "The unique identifier of the edge to retrieve.",
					},
				},
				Required: []string{"id"},
			},
		},
		{
			Name:        "list_edges",
			Description: ptr("Lists graph edges based on optional filters. Can filter by edge type, source node ID, and/or target node ID."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Type: anthropic.ToolInputSchemaTypeObject,
				Properties: map[string]interface{}{
					"type": map[string]interface{}{
						"type":        "string",
						"description": "Filter edges by this type. For example, 'knows', 'contains', etc.",
					},
					"from_id": map[string]interface{}{
						"type":        "integer",
						"description": "Filter edges that originate from this node ID.",
					},
					"to_id": map[string]interface{}{
						"type":        "integer",
						"description": "Filter edges that point to this node ID.",
					},
				},
			},
		},
		{
			Name:        "create_edge",
			Description: ptr("Creates a new graph edge connecting two nodes. Specify the edge type and the IDs of the source and target nodes."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Type: anthropic.ToolInputSchemaTypeObject,
				Properties: map[string]interface{}{
					"type": map[string]interface{}{
						"type":        "string",
						"description": "The type of the edge to create. For example, 'knows', 'contains', etc.",
					},
					"from_id": map[string]interface{}{
						"type":        "integer",
						"description": "The ID of the source node where the edge starts.",
					},
					"to_id": map[string]interface{}{
						"type":        "integer",
						"description": "The ID of the target node where the edge ends.",
					},
				},
				Required: []string{"type", "from_id", "to_id"},
			},
		},
		{
			Name:        "delete_edge",
			Description: ptr("Deletes a graph edge by its ID."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Type: anthropic.ToolInputSchemaTypeObject,
				Properties: map[string]interface{}{
					"id": map[string]interface{}{
						"type":        "integer",
						"description": "The unique identifier of the edge to delete.",
					},
				},
				Required: []string{"id"},
			},
		},
	}
}

func (a *agent) handleToolCall(m *Message) error {
	if m.MType != "tool" || m.ToolMsg == nil {
		return fmt.Errorf("not a tool message")
	}

	// Mark as handled
	m.ToolMsg.ToolDone = true

	var err error
	var result any

	fmt.Fprintf(os.Stderr, "Handling tool call %q\n", m.ToolMsg.ToolName)

	switch m.ToolMsg.ToolName {
	case "get_node":
		id, ok := m.ToolMsg.ToolArgs["id"].(float64)
		if !ok {
			return fmt.Errorf("invalid id parameter")
		}
		result, err = a.c.GetNode(int(id))

	case "list_nodes":
		nodeType, _ := m.ToolMsg.ToolArgs["node_type"].(string)
		result, err = a.c.ListNodes(nodeType)

	case "create_node":
		typ, ok := m.ToolMsg.ToolArgs["type"].(string)
		if !ok {
			return fmt.Errorf("invalid type parameter")
		}
		props, _ := m.ToolMsg.ToolArgs["props"].(map[string]any)
		result, err = a.c.CreateNode(typ, props)

	case "delete_node":
		id, ok := m.ToolMsg.ToolArgs["id"].(float64)
		if !ok {
			return fmt.Errorf("invalid id parameter")
		}
		err = a.c.DeleteNode(int(id))
		if err == nil {
			result = map[string]bool{"success": true}
		}

	case "get_edge":
		id, ok := m.ToolMsg.ToolArgs["id"].(float64)
		if !ok {
			return fmt.Errorf("invalid id parameter")
		}
		result, err = a.c.GetEdge(int(id))

	case "list_edges":
		filter := EdgeFilter{}
		if typ, ok := m.ToolMsg.ToolArgs["type"].(string); ok {
			filter.Type = typ
		}
		if fromID, ok := m.ToolMsg.ToolArgs["from_id"].(float64); ok {
			filter.FromID = int(fromID)
		}
		if toID, ok := m.ToolMsg.ToolArgs["to_id"].(float64); ok {
			filter.ToID = int(toID)
		}
		result, err = a.c.ListEdges(filter)

	case "create_edge":
		typ, ok := m.ToolMsg.ToolArgs["type"].(string)
		if !ok {
			return fmt.Errorf("invalid type parameter")
		}
		fromID, ok := m.ToolMsg.ToolArgs["from_id"].(float64)
		if !ok {
			return fmt.Errorf("invalid from_id parameter")
		}
		toID, ok := m.ToolMsg.ToolArgs["to_id"].(float64)
		if !ok {
			return fmt.Errorf("invalid to_id parameter")
		}
		result, err = a.c.CreateEdge(typ, int(fromID), int(toID))

	case "delete_edge":
		id, ok := m.ToolMsg.ToolArgs["id"].(float64)
		if !ok {
			return fmt.Errorf("invalid id parameter")
		}
		err = a.c.DeleteEdge(int(id))
		if err == nil {
			result = map[string]bool{"success": true}
		}

	default:
		return fmt.Errorf("unknown tool: %s", m.ToolMsg.ToolName)
	}

	if err != nil {
		m.ToolMsg.ToolError = err.Error()
		return a.c.UpdateMessage(*m)
	}

	// Convert result to JSON-encoded string
	jsonResult, err := json.Marshal(result)
	if err != nil {
		m.ToolMsg.ToolError = fmt.Sprintf("failed to encode result: %v", err)
		return a.c.UpdateMessage(*m)
	}

	m.ToolMsg.ToolResult = string(jsonResult)
	return a.c.UpdateMessage(*m)
}

// Generic helper function to create a pointer
func ptr[T any](v T) *T {
	return &v
}
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	ollama "github.com/ollama/ollama/api"
)

const defaultModel = "qwen3"

type agent struct {
	ol *ollama.Client
	c  *client
	gc chan struct{ cid int }
}

func newAgent(ctx context.Context, c *client) (*agent, error) {
	// Connect to the ollama client
	ol, err := ollama.ClientFromEnvironment()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to ollama: %w", err)
	}

	// Check that ollama is running
	if err := ol.Heartbeat(ctx); err != nil {
		return nil, fmt.Errorf("ollama is not running: %w", err)
	}
	return &agent{
		ol: ol,
		c:  c,
		gc: make(chan struct{ cid int }),
	}, nil
}

func (a *agent) getChatHistory(cid int) ([]ollama.Message, error) {
	// Get the messages in the chat
	ms, err := a.c.ListMessages(cid)
	if err != nil {
		return nil, fmt.Errorf("failed to list messages: %w", err)
	}

	// Convert them to ollama messages
	var hs []ollama.Message
	for _, m := range ms {
		switch m.MType {
		case "user":
			hs = append(hs, ollama.Message{
				Role:    "user",
				Content: m.UserMsg.Text,
			})
		case "agent":
			hs = append(hs, ollama.Message{
				Role:    "assistant",
				Content: m.AgentMsg.Text,
			})
		case "tool":
			// NOTE: Tool calls internally are one message
			// but to ollama they're two – the agent's call
			// and the tool's response.
			hs = append(hs, ollama.Message{
				Role: "assistant",
				ToolCalls: []ollama.ToolCall{
					{Function: ollama.ToolCallFunction{
						Index:     0, // TODO: Change this?
						Name:      m.ToolMsg.ToolName,
						Arguments: m.ToolMsg.ToolArgs,
					}},
				},
			})
			hs = append(hs, ollama.Message{
				Role:    "tool",
				Content: m.ToolMsg.ToolResult,
			})
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

	// Generate a response using ollama
	var m *Message
	if err := a.ol.Chat(ctx, &ollama.ChatRequest{
		Model:    defaultModel,
		Messages: h,
		Stream:   new(bool), // Always false
		Tools:    a.getTools(),
	}, func(resp ollama.ChatResponse) error {
		// Trigger when done
		defer onupdate()

		// Has the message already been created? Then update it.
		if m != nil && m.MType == "agent" {
			m.AgentMsg.Text += resp.Message.Content
			return a.c.UpdateMessage(*m)
		}
		if m != nil && m.MType == "tool" {
			fmt.Fprintf(os.Stderr, "Getting tool call update: %#v\n", resp)
			// m.ToolMsg.ToolName += resp.Message.Content
			// return a.c.UpdateMessage(*m)
		}

		// Create a message based on the response
		m = &Message{
			ChatID: cid,
			// MessageID: 0, // Intentionally not set
			MType: "agent",
			AgentMsg: &struct{ Text string }{
				Text: resp.Message.Content,
			},
		}
		if len(resp.Message.ToolCalls) > 0 {
			fmt.Fprintln(os.Stderr, "Creating tool call: "+resp.Message.ToolCalls[0].Function.Name)
			m.MType = "tool"
			m.ToolMsg = &struct {
				ToolDone   bool
				ToolName   string
				ToolArgs   map[string]any
				ToolResult string
				ToolError  string
			}{
				ToolDone: resp.Done,
				ToolName: resp.Message.ToolCalls[0].Function.Name,
				ToolArgs: resp.Message.ToolCalls[0].Function.Arguments,
			}
		}

		// Create the message
		msg, err := a.c.CreateMessage(*m)
		if err != nil {
			return fmt.Errorf("failed to create message: %w", err)
		}
		m = msg
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to generate response: %w", err)
	}

	// Handle the tool call
	if m.MType == "tool" && m.ToolMsg.ToolDone {
		fmt.Fprintln(os.Stderr, "Calling the tool")
		// NOTE: This will update the message in the client
		if err := a.handleToolCall(m); err != nil {
			return nil, fmt.Errorf("failed to handle tool call: %w", err)
		}
	}
	return m, nil
}

func (a *agent) getTools() []ollama.Tool {
	return []ollama.Tool{
		{
			Type: "function",
			Function: ollama.ToolFunction{
				Name:        "get_node",
				Description: "Retrieves a single graph node by its ID. Returns the node's ID, type, and properties.",
				Parameters: struct {
					Type       string   `json:"type"`
					Defs       any      `json:"$defs,omitempty"`
					Items      any      `json:"items,omitempty"`
					Required   []string `json:"required"`
					Properties map[string]struct {
						Type        ollama.PropertyType `json:"type"`
						Items       any                 `json:"items,omitempty"`
						Description string              `json:"description"`
						Enum        []any               `json:"enum,omitempty"`
					} `json:"properties"`
				}{
					Type:     "object",
					Required: []string{"id"},
					Properties: map[string]struct {
						Type        ollama.PropertyType `json:"type"`
						Items       any                 `json:"items,omitempty"`
						Description string              `json:"description"`
						Enum        []any               `json:"enum,omitempty"`
					}{
						"id": {
							Type:        []string{"integer"},
							Description: "The unique identifier of the node to retrieve.",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: ollama.ToolFunction{
				Name:        "list_nodes",
				Description: "Lists all graph nodes of a specific type. If no type is provided, returns all nodes.",
				Parameters: struct {
					Type       string   `json:"type"`
					Defs       any      `json:"$defs,omitempty"`
					Items      any      `json:"items,omitempty"`
					Required   []string `json:"required"`
					Properties map[string]struct {
						Type        ollama.PropertyType `json:"type"`
						Items       any                 `json:"items,omitempty"`
						Description string              `json:"description"`
						Enum        []any               `json:"enum,omitempty"`
					} `json:"properties"`
				}{
					Type: "object",
					Properties: map[string]struct {
						Type        ollama.PropertyType `json:"type"`
						Items       any                 `json:"items,omitempty"`
						Description string              `json:"description"`
						Enum        []any               `json:"enum,omitempty"`
					}{
						"node_type": {
							Type:        []string{"string"},
							Description: "The type of nodes to list. If empty, all nodes will be returned.",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: ollama.ToolFunction{
				Name:        "create_node",
				Description: "Creates a new graph node with the specified type and properties. Returns the created node with its assigned ID.",
				Parameters: struct {
					Type       string   `json:"type"`
					Defs       any      `json:"$defs,omitempty"`
					Items      any      `json:"items,omitempty"`
					Required   []string `json:"required"`
					Properties map[string]struct {
						Type        ollama.PropertyType `json:"type"`
						Items       any                 `json:"items,omitempty"`
						Description string              `json:"description"`
						Enum        []any               `json:"enum,omitempty"`
					} `json:"properties"`
				}{
					Type:     "object",
					Required: []string{"type"},
					Properties: map[string]struct {
						Type        ollama.PropertyType `json:"type"`
						Items       any                 `json:"items,omitempty"`
						Description string              `json:"description"`
						Enum        []any               `json:"enum,omitempty"`
					}{
						"type": {
							Type:        []string{"string"},
							Description: "The type of the node to create. For example, 'person', 'document', etc.",
						},
						"props": {
							Type:        []string{"object"},
							Description: "A map of properties to store with the node. For example, {\"name\": \"John\", \"age\": 30}.",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: ollama.ToolFunction{
				Name:        "delete_node",
				Description: "Deletes a graph node by its ID. Note that this will also delete all edges connected to this node.",
				Parameters: struct {
					Type       string   `json:"type"`
					Defs       any      `json:"$defs,omitempty"`
					Items      any      `json:"items,omitempty"`
					Required   []string `json:"required"`
					Properties map[string]struct {
						Type        ollama.PropertyType `json:"type"`
						Items       any                 `json:"items,omitempty"`
						Description string              `json:"description"`
						Enum        []any               `json:"enum,omitempty"`
					} `json:"properties"`
				}{
					Type:     "object",
					Required: []string{"id"},
					Properties: map[string]struct {
						Type        ollama.PropertyType `json:"type"`
						Items       any                 `json:"items,omitempty"`
						Description string              `json:"description"`
						Enum        []any               `json:"enum,omitempty"`
					}{
						"id": {
							Type:        []string{"integer"},
							Description: "The unique identifier of the node to delete.",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: ollama.ToolFunction{
				Name:        "get_edge",
				Description: "Retrieves a single graph edge by its ID. Returns the edge's ID, type, and the IDs of its connected nodes.",
				Parameters: struct {
					Type       string   `json:"type"`
					Defs       any      `json:"$defs,omitempty"`
					Items      any      `json:"items,omitempty"`
					Required   []string `json:"required"`
					Properties map[string]struct {
						Type        ollama.PropertyType `json:"type"`
						Items       any                 `json:"items,omitempty"`
						Description string              `json:"description"`
						Enum        []any               `json:"enum,omitempty"`
					} `json:"properties"`
				}{
					Type:     "object",
					Required: []string{"id"},
					Properties: map[string]struct {
						Type        ollama.PropertyType `json:"type"`
						Items       any                 `json:"items,omitempty"`
						Description string              `json:"description"`
						Enum        []any               `json:"enum,omitempty"`
					}{
						"id": {
							Type:        []string{"integer"},
							Description: "The unique identifier of the edge to retrieve.",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: ollama.ToolFunction{
				Name:        "list_edges",
				Description: "Lists graph edges based on optional filters. Can filter by edge type, source node ID, and/or target node ID.",
				Parameters: struct {
					Type       string   `json:"type"`
					Defs       any      `json:"$defs,omitempty"`
					Items      any      `json:"items,omitempty"`
					Required   []string `json:"required"`
					Properties map[string]struct {
						Type        ollama.PropertyType `json:"type"`
						Items       any                 `json:"items,omitempty"`
						Description string              `json:"description"`
						Enum        []any               `json:"enum,omitempty"`
					} `json:"properties"`
				}{
					Type: "object",
					Properties: map[string]struct {
						Type        ollama.PropertyType `json:"type"`
						Items       any                 `json:"items,omitempty"`
						Description string              `json:"description"`
						Enum        []any               `json:"enum,omitempty"`
					}{
						"type": {
							Type:        []string{"string"},
							Description: "Filter edges by this type. For example, 'knows', 'contains', etc.",
						},
						"from_id": {
							Type:        []string{"integer"},
							Description: "Filter edges that originate from this node ID.",
						},
						"to_id": {
							Type:        []string{"integer"},
							Description: "Filter edges that point to this node ID.",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: ollama.ToolFunction{
				Name:        "create_edge",
				Description: "Creates a new graph edge connecting two nodes. Specify the edge type and the IDs of the source and target nodes.",
				Parameters: struct {
					Type       string   `json:"type"`
					Defs       any      `json:"$defs,omitempty"`
					Items      any      `json:"items,omitempty"`
					Required   []string `json:"required"`
					Properties map[string]struct {
						Type        ollama.PropertyType `json:"type"`
						Items       any                 `json:"items,omitempty"`
						Description string              `json:"description"`
						Enum        []any               `json:"enum,omitempty"`
					} `json:"properties"`
				}{
					Type:     "object",
					Required: []string{"type", "from_id", "to_id"},
					Properties: map[string]struct {
						Type        ollama.PropertyType `json:"type"`
						Items       any                 `json:"items,omitempty"`
						Description string              `json:"description"`
						Enum        []any               `json:"enum,omitempty"`
					}{
						"type": {
							Type:        []string{"string"},
							Description: "The type of the edge to create. For example, 'knows', 'contains', etc.",
						},
						"from_id": {
							Type:        []string{"integer"},
							Description: "The ID of the source node where the edge starts.",
						},
						"to_id": {
							Type:        []string{"integer"},
							Description: "The ID of the target node where the edge ends.",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: ollama.ToolFunction{
				Name:        "delete_edge",
				Description: "Deletes a graph edge by its ID.",
				Parameters: struct {
					Type       string   `json:"type"`
					Defs       any      `json:"$defs,omitempty"`
					Items      any      `json:"items,omitempty"`
					Required   []string `json:"required"`
					Properties map[string]struct {
						Type        ollama.PropertyType `json:"type"`
						Items       any                 `json:"items,omitempty"`
						Description string              `json:"description"`
						Enum        []any               `json:"enum,omitempty"`
					} `json:"properties"`
				}{
					Type:     "object",
					Required: []string{"id"},
					Properties: map[string]struct {
						Type        ollama.PropertyType `json:"type"`
						Items       any                 `json:"items,omitempty"`
						Description string              `json:"description"`
						Enum        []any               `json:"enum,omitempty"`
					}{
						"id": {
							Type:        []string{"integer"},
							Description: "The unique identifier of the edge to delete.",
						},
					},
				},
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

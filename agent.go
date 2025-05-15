package main

import (
	"context"
	"fmt"

	ollama "github.com/ollama/ollama/api"
)

const defaultModel = "qwen3"

type agent struct {
	ol *ollama.Client
	c  *client
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
	}, nil
}

func (a *agent) getChatHistory(cid int) ([]ollama.Message, error) {
	ms, err := a.c.ListMessages(cid)
	if err != nil {
		return nil, fmt.Errorf("failed to list messages: %w", err)
	}

	hs := make([]ollama.Message, len(ms))
	for i, m := range ms {
		hs[i] = ollama.Message{
			// TODO: Finish filling this in...
		}
	}
	return hs, nil
}

func (a *agent) generate(ctx context.Context, cid int) error {
	// Get the previous messages from the conversation
	h, err := a.getChatHistory(cid)
	if err != nil {
		return fmt.Errorf("failed to get messages: %w", err)
	}

	// Generate a response using ollama
	var stream bool
	if err := a.ol.Chat(ctx, &ollama.ChatRequest{
		Model:    defaultModel,
		Messages: h,
		Stream:   &stream, // TODO: Allow streaming...
		Tools: []ollama.Tool{
			{
				// TODO: Finish filling this in...
				Type: "function",
				Function: ollama.ToolFunction{
					Name:        "TK",
					Description: "",
					// Parameters:  struct{}{},
				},
			},
		},
	}, func(resp ollama.ChatResponse) error {
		// TODO: Finish filling this in...
		return nil
	}); err != nil {
		return fmt.Errorf("failed to generate response: %w", err)
	}

	// Send the response to the client
	if err := a.c.sendMessage(ctx, cid, resp.Text); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

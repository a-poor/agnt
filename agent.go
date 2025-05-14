package main

import (
	"context"
	"fmt"

	ollama "github.com/ollama/ollama/api"
)

type Agent struct {
	ol *ollama.Client
}

func newAgent(ctx context.Context) (*Agent, error) {
	// Connect to the ollama client
	c, err := ollama.ClientFromEnvironment()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to ollama: %w", err)
	}

	// Check that ollama is running
	if err := c.Heartbeat(ctx); err != nil {
		return nil, fmt.Errorf("ollama is not running: %w", err)
	}
	return &Agent{
		ol: c,
	}, nil
}

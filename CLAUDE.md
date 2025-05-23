# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Run Commands

**Build the application:**
```bash
go build -o agnt
```

**Run the application:**
```bash
go run .
```

**Initialize with test chat (temporary flag):**
```bash
go run . --init
```

**Test and format:**
```bash
go test ./...
go fmt ./...
go vet ./...
```

## Architecture Overview

This is a Go-based AI agent application with a terminal UI that combines chat functionality with graph database operations. The architecture consists of:

### Core Components

- **main.go**: Entry point that creates and runs the CLI application
- **app.go**: CLI setup using urfave/cli/v3, initializes client and agent, runs TUI
- **client.go**: Database layer using BoltDB for persistent storage of chats, messages, and graph data
- **agent.go**: AI integration layer that connects to Ollama for LLM interactions with tool calling
- **model.go**: TUI implementation using Charmbracelet Bubbletea with viewport and textarea

### Data Models

The application manages three main data types:
1. **Chat System**: `ChatInfo` and `Message` structs for conversation management
2. **Graph Database**: `GraphNode` and `GraphEdge` structs for knowledge graph operations
3. **Message Types**: "user", "agent", and "tool" messages with different payload structures

### Storage Architecture

Uses BoltDB with bucket-based organization:
- `chats`: Chat metadata
- `#MESSAGES#{chatID}`: Per-chat message buckets
- `graph:nodes`: Graph nodes storage
- `graph:edges`: Graph edges storage
- `__meta`: Schema versioning and metadata

### LLM Integration

The agent connects to Ollama and provides 8 predefined tools for graph operations:
- Node operations: get_node, list_nodes, create_node, delete_node
- Edge operations: get_edge, list_edges, create_edge, delete_edge

Tool calls are handled as a special message type that includes function name, arguments, and results.

### TUI Architecture

The interface uses two main components:
- **Viewport**: Displays chat history with different styling for user/agent/tool messages
- **Textarea**: Input area for user messages

Focus can be switched between components using Tab, and Enter sends messages or scrolls viewport.

## Key Implementation Details

- Database path: `~/.agnt/agnt.db`
- Default LLM model: "qwen3" (configurable via `defaultModel` constant)
- Message flow: User input → Database storage → Agent generation → Tool execution → Database update → UI refresh
- Graph operations maintain referential integrity (deleting nodes removes connected edges)
- Tool calls are synchronous and update the message in-place with results

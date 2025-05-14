package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path"

	ollama "github.com/ollama/ollama/api"
	bolt "go.etcd.io/bbolt"
)

const (
	confDir       = ".agnt"
	dbFile        = "agnt.db"
	schemaVersion = "v1"
	metaBucket    = "__meta"
	versionKey    = "version"
	chatBucket    = "chats"
	messageBucket = "messages"
	nodeBucket    = "graph:nodes"
	edgeBucket    = "graph:edges"
	defaultModel  = "qwen3"
)

// client manages state
type client struct {
	dbp string
	db  *bolt.DB
	ol  *ollama.Client
}

func newClient(ctx context.Context, d string) (*client, error) {
	// Format the config dir
	p := path.Join(d, confDir)

	// Make the config directory if it doesn't exist
	if err := os.MkdirAll(p, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	// Add the db name
	p = path.Join(p, dbFile)

	// Open the bolt database
	db, err := bolt.Open(p, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Check if the version key is set and if
	// updates need to be run
	if err := db.Update(func(tx *bolt.Tx) error {
		// Get the bucket
		b, err := tx.CreateBucketIfNotExists([]byte(metaBucket))
		if err != nil {
			return fmt.Errorf("failed to get/create meta bucket: %w", err)
		}

		// Get the current version
		switch v := b.Get([]byte(versionKey)); string(v) {
		case "":
			// Not set? Initialize the database
			if err := b.Put([]byte(versionKey), []byte(versionKey)); err != nil {
				return fmt.Errorf("failed to set version key: %w", err)
			}

			// Create the chat bucket
			if _, err := tx.CreateBucket([]byte(chatBucket)); err != nil {
				return fmt.Errorf("failed to create meta bucket: %w", err)
			}

			// Create the node bucket
			if _, err := tx.CreateBucket([]byte(nodeBucket)); err != nil {
				return fmt.Errorf("failed to create graph node bucket: %w", err)
			}

			// Create the edge bucket
			if _, err := tx.CreateBucket([]byte(edgeBucket)); err != nil {
				return fmt.Errorf("failed to create graph edge bucket: %w", err)
			}

			return nil

		case versionKey:
			// Already set? No need to do anything
			return nil

		default:
			// Unknown! Stop here.
			return fmt.Errorf("unknown version %q", string(v))
		}
	}); err != nil {
		return nil, fmt.Errorf("failed to update database: %w", err)
	}

	// Connect to the ollama client
	c, err := ollama.ClientFromEnvironment()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to ollama: %w", err)
	}

	// Check that ollama is running
	if err := c.Heartbeat(ctx); err != nil {
		return nil, fmt.Errorf("ollama is not running: %w", err)
	}

	// TODO: Set up channels?
	// ...

	// Return the client
	return &client{
		dbp: p,
		db:  db,
		ol:  c,
	}, nil
}

func (c *client) Close() error {
	if err := c.db.Close(); err != nil {
		return fmt.Errorf("failed to close database: %w", err)
	}

	return nil
}

type ChatInfo struct {
	ID   int
	Name string
}

func (ci ChatInfo) BID() []byte {
	return itob(ci.ID)
}

func (ci ChatInfo) MessageBucketName() []byte {
	return append([]byte(`#MESSAGES#`), itob(ci.ID)...)
}

// ListChats retrieves all chat threads from the database.
func (c *client) ListChats() ([]ChatInfo, error) {
	var chats []ChatInfo
	if err := c.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket([]byte(chatBucket)).Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var ci ChatInfo
			if err := json.Unmarshal(v, &ci); err != nil {
				return fmt.Errorf("failed to unmarshal chat info: %w", err)
			}
			chats = append(chats, ci)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to read from db: %w", err)
	}
	return chats, nil
}

// CreateChat adds a new chat thread to the database.
func (c *client) CreateChat(n string) (*ChatInfo, error) {
	var ci *ChatInfo
	if err := c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(chatBucket))
		id, err := b.NextSequence()
		if err != nil {
			return fmt.Errorf("failed to get next sequence: %w", err)
		}

		// Define the new object and marshall it
		ci = &ChatInfo{
			ID:   int(id),
			Name: n,
		}
		by, err := json.Marshal(ci)
		if err != nil {
			return fmt.Errorf("failed to marshal chat info as json: %w", err)
		}

		// Store it in the database
		if err := b.Put(ci.BID(), by); err != nil {
			return fmt.Errorf("failed to put chat info into db: %w", err)
		}

		// Create a new bucket for the chat messages
		if _, err := tx.CreateBucket(ci.MessageBucketName()); err != nil {
			return fmt.Errorf("failed to create chat messages bucket: %w", err)
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to read from db: %w", err)
	}
	return ci, nil
}

// DeleteChat removes a chat thread from the database.
func (c *client) DeleteChat(id int) error {
	if err := c.db.Update(func(tx *bolt.Tx) error {
		// Delete the record in the chat bucket
		b := tx.Bucket([]byte(chatBucket))
		if err := b.Delete(itob(id)); err != nil {
			return fmt.Errorf("failed to delete chat info from db: %w", err)
		}

		// Delete the whole chat's message bucket
		if err := tx.DeleteBucket(ChatInfo{ID: id}.MessageBucketName()); err != nil {
			return fmt.Errorf("failed to delete chat messages bucket: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to delete chat from db: %w", err)
	}
	return nil
}

type Message struct {
	ChatID    int
	MessageID int
	MType     string // "user" | "agent" | "tool"
	UserMsg   *struct {
		Text string // The text the user sent
	}
	AgentMsg *struct {
		Text string // The text the agent sent
	}
	ToolMsg *struct {
		ToolName   string // Name of the tool called
		ToolArgs   string // The arguments passed to the tool
		ToolResult string // The result of the tool call
		ToolError  string // The error message if the tool call failed
	}
}

func (m Message) BID() []byte {
	return itob(m.MessageID)
}

// ListMessages retrieves all messages for a specific chat from the database.
func (c *client) ListMessages() {}

// CreateMessage adds a new message to a chat thread in the database.
func (c *client) CreateMessage(context.Context) {}

// DeleteMessage removes a message from the database.
func (c *client) DeleteMessage(context.Context) {}

// ListNodes retrieves all nodes from the graph database.
func (c *client) ListNodes(context.Context) {}

// CreateNode adds a new node to the graph database.
func (c *client) CreateNode(context.Context) {}

// DeleteNode removes a node from the graph database.
func (c *client) DeleteNode(context.Context) {}

// ListEdges retrieves all edges from the graph database.
func (c *client) ListEdges(context.Context) {}

// CreateEdge adds a new edge to the graph database.
func (c *client) CreateEdge(context.Context) {}

// DeleteEdge removes an edge from the graph database.
func (c *client) DeleteEdge(context.Context) {}

func itob(i int) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(i))
	return b
}

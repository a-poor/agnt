package main

import (
	"context"
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

			// Create the message bucket
			if _, err := tx.CreateBucket([]byte(messageBucket)); err != nil {
				return fmt.Errorf("failed to create message bucket: %w", err)
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

func (c *client) ListChats(context.Context) {}

func (c *client) CreateChat(context.Context) {}

func (c *client) DeleteChat(context.Context) {}

func (c *client) ListMessages(context.Context) {}

func (c *client) SendMessage(context.Context) {}

func (c *client) DeleteMessage(context.Context) {}

func (c *client) ListNodes(context.Context) {}

func (c *client) CreateNode(context.Context) {}

func (c *client) DeleteNode(context.Context) {}

func (c *client) ListEdges(context.Context) {}

func (c *client) CreateEdge(context.Context) {}

func (c *client) DeleteEdge(context.Context) {}

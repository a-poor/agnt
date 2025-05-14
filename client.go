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
)

// client manages state
type client struct {
	dbp string
	db  *bolt.DB
	ol  *ollama.Client
}

func newClient(ctx context.Context, hd string) (*client, error) {
	// Format the config dir
	p := path.Join(hd, confDir)

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
		v := b.Get([]byte(versionKey))

		switch string(v) {
		case "":
			// Not set? Initialize the database
			if err := b.Put([]byte(versionKey), []byte(versionKey)); err != nil {
				return fmt.Errorf("failed to set version key: %w", err)
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

	// Return the client
	return &client{
		dbp: p,
		db:  db,
		ol:  c,
	}, nil
}

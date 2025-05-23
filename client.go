package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path"

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
)

// client manages state
type client struct {
	dbp string
	db  *bolt.DB
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

	// TODO: Set up channels?
	// ...

	// Return the client
	return &client{
		dbp: p,
		db:  db,
	}, nil
}

func (c *client) Close() error {
	if err := c.db.Close(); err != nil {
		return fmt.Errorf("failed to close database: %w", err)
	}

	return nil
}

type ChatInfo struct {
	ID    int
	Name  string
	State string // "idle" or "running"
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
			ID:    int(id),
			Name:  n,
			State: "idle",
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

// UpdateChatState updates the state of a chat thread ("idle" or "running").
func (c *client) UpdateChatState(chatID int, state string) error {
	if state != "idle" && state != "running" {
		return fmt.Errorf("invalid state: %s (must be 'idle' or 'running')", state)
	}
	
	return c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(chatBucket))
		
		// Get existing chat
		v := b.Get(itob(chatID))
		if v == nil {
			return fmt.Errorf("chat not found: %d", chatID)
		}
		
		// Unmarshal existing chat
		var ci ChatInfo
		if err := json.Unmarshal(v, &ci); err != nil {
			return fmt.Errorf("failed to unmarshal chat info: %w", err)
		}
		
		// Update state
		ci.State = state
		
		// Marshal and save back
		by, err := json.Marshal(ci)
		if err != nil {
			return fmt.Errorf("failed to marshal chat info: %w", err)
		}
		
		if err := b.Put(ci.BID(), by); err != nil {
			return fmt.Errorf("failed to update chat in db: %w", err)
		}
		
		return nil
	})
}

// GetChat retrieves a single chat by ID.
func (c *client) GetChat(chatID int) (*ChatInfo, error) {
	var ci *ChatInfo
	err := c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(chatBucket))
		v := b.Get(itob(chatID))
		if v == nil {
			return fmt.Errorf("chat not found: %d", chatID)
		}
		
		var chat ChatInfo
		if err := json.Unmarshal(v, &chat); err != nil {
			return fmt.Errorf("failed to unmarshal chat info: %w", err)
		}
		ci = &chat
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ci, nil
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
		ToolDone   bool
		ToolName   string         // Name of the tool called
		ToolArgs   map[string]any // The arguments passed to the tool
		ToolResult string         // The result of the tool call
		ToolError  string         // The error message if the tool call failed
	}
}

func (m Message) BID() []byte {
	return itob(m.MessageID)
}

// ListMessages retrieves all messages for a specific chat from the database.
func (c *client) ListMessages(chatID int) ([]Message, error) {
	var msgs []Message
	if err := c.db.View(func(tx *bolt.Tx) error {
		bucketName := ChatInfo{ID: chatID}.MessageBucketName()
		bucket := tx.Bucket(bucketName)
		if bucket == nil {
			return fmt.Errorf("chat messages bucket not found")
		}

		cursor := bucket.Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			var msg Message
			if err := json.Unmarshal(v, &msg); err != nil {
				return fmt.Errorf("failed to unmarshal message: %w", err)
			}
			msgs = append(msgs, msg)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to read messages from db: %w", err)
	}
	return msgs, nil
}

// CreateMessage adds a new message to a chat thread in the database.
func (c *client) CreateMessage(msg Message) (*Message, error) {
	if err := c.db.Update(func(tx *bolt.Tx) error {
		bucketName := ChatInfo{ID: msg.ChatID}.MessageBucketName()
		bucket := tx.Bucket(bucketName)
		if bucket == nil {
			return fmt.Errorf("chat messages bucket not found")
		}

		// Get next sequence for message ID
		id, err := bucket.NextSequence()
		if err != nil {
			return fmt.Errorf("failed to get next sequence: %w", err)
		}

		// Set the message ID
		msg.MessageID = int(id)

		// Marshal the message
		data, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("failed to marshal message: %w", err)
		}

		// Store it in the database
		if err := bucket.Put(msg.BID(), data); err != nil {
			return fmt.Errorf("failed to put message into db: %w", err)
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to create message: %w", err)
	}

	return &msg, nil
}

func (c *client) GetMessage(cid, mid int) (*Message, error) {
	var msg Message
	if err := c.db.View(func(tx *bolt.Tx) error {
		bucketName := ChatInfo{ID: cid}.MessageBucketName()
		bucket := tx.Bucket(bucketName)
		if bucket == nil {
			return fmt.Errorf("chat messages bucket not found")
		}

		data := bucket.Get(itob(mid))
		if data == nil {
			return fmt.Errorf("message not found")
		}

		if err := json.Unmarshal(data, &msg); err != nil {
			return fmt.Errorf("failed to unmarshal message: %w", err)
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to get message: %w", err)
	}
	return &msg, nil
}

func (c *client) UpdateMessage(msg Message) error {
	if err := c.db.Update(func(tx *bolt.Tx) error {
		bucketName := ChatInfo{ID: msg.ChatID}.MessageBucketName()
		bucket := tx.Bucket(bucketName)
		if bucket == nil {
			return fmt.Errorf("chat messages bucket not found")
		}

		data, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("failed to marshal message: %w", err)
		}

		if err := bucket.Put(itob(msg.MessageID), data); err != nil {
			return fmt.Errorf("failed to put message into db: %w", err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("failed to update message: %w", err)
	}

	return nil
}

// DeleteMessage removes a message from the database.
func (c *client) DeleteMessage(chatID, messageID int) error {
	if err := c.db.Update(func(tx *bolt.Tx) error {
		bucketName := ChatInfo{ID: chatID}.MessageBucketName()
		bucket := tx.Bucket(bucketName)
		if bucket == nil {
			return fmt.Errorf("chat messages bucket not found")
		}

		if err := bucket.Delete(itob(messageID)); err != nil {
			return fmt.Errorf("failed to delete message from db: %w", err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("failed to delete message: %w", err)
	}

	return nil
}

type GraphNode struct {
	ID    int
	Type  string
	Props map[string]any
}

func (n GraphNode) BID() []byte {
	return itob(n.ID)
}

// GetNode retrieves a node by its ID from the graph database.
func (c *client) GetNode(id int) (*GraphNode, error) {
	var node *GraphNode
	if err := c.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(nodeBucket))
		if bucket == nil {
			return fmt.Errorf("node bucket not found")
		}

		data := bucket.Get(itob(id))
		if data == nil {
			return fmt.Errorf("node with ID %d not found", id)
		}

		node = &GraphNode{}
		if err := json.Unmarshal(data, node); err != nil {
			return fmt.Errorf("failed to unmarshal node: %w", err)
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to get node: %w", err)
	}

	return node, nil
}

// ListNodes retrieves all nodes from the graph database.
func (c *client) ListNodes(nodeType string) ([]GraphNode, error) {
	var nodes []GraphNode
	if err := c.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(nodeBucket))
		if bucket == nil {
			return fmt.Errorf("node bucket not found")
		}

		cursor := bucket.Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			var node GraphNode
			if err := json.Unmarshal(v, &node); err != nil {
				return fmt.Errorf("failed to unmarshal node: %w", err)
			}

			// Apply type filter if specified
			if nodeType != "" && node.Type != nodeType {
				continue
			}

			nodes = append(nodes, node)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}
	return nodes, nil
}

// CreateNode adds a new node to the graph database.
func (c *client) CreateNode(nodeType string, props map[string]any) (*GraphNode, error) {
	var node *GraphNode
	if err := c.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(nodeBucket))
		if bucket == nil {
			return fmt.Errorf("node bucket not found")
		}

		// Get next sequence for node ID
		id, err := bucket.NextSequence()
		if err != nil {
			return fmt.Errorf("failed to get next sequence: %w", err)
		}

		// Create the node
		node = &GraphNode{
			ID:    int(id),
			Type:  nodeType,
			Props: props,
		}

		// Marshal the node
		data, err := json.Marshal(node)
		if err != nil {
			return fmt.Errorf("failed to marshal node: %w", err)
		}

		// Store it in the database
		if err := bucket.Put(node.BID(), data); err != nil {
			return fmt.Errorf("failed to put node into db: %w", err)
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to create node: %w", err)
	}

	return node, nil
}

// DeleteNode removes a node from the graph database.
func (c *client) DeleteNode(id int) error {
	if err := c.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(nodeBucket))
		if bucket == nil {
			return fmt.Errorf("node bucket not found")
		}

		if err := bucket.Delete(itob(id)); err != nil {
			return fmt.Errorf("failed to delete node from db: %w", err)
		}

		// Also delete any related edges
		edgeBucket := tx.Bucket([]byte(edgeBucket))
		if edgeBucket == nil {
			return fmt.Errorf("edge bucket not found")
		}

		// Iterate through all edges and delete those connected to this node
		cursor := edgeBucket.Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			var edge GraphEdge
			if err := json.Unmarshal(v, &edge); err != nil {
				return fmt.Errorf("failed to unmarshal edge: %w", err)
			}

			if edge.FromID == id || edge.ToID == id {
				if err := edgeBucket.Delete(k); err != nil {
					return fmt.Errorf("failed to delete related edge: %w", err)
				}
			}
		}

		return nil
	}); err != nil {
		return fmt.Errorf("failed to delete node: %w", err)
	}

	return nil
}

type GraphEdge struct {
	ID     int
	Type   string
	FromID int
	ToID   int
}

func (e GraphEdge) BID() []byte {
	return itob(e.ID)
}

// EdgeFilter provides criteria for filtering edges in ListEdges.
type EdgeFilter struct {
	Type   string // Filter by edge type (if not empty)
	FromID int    // Filter by source node ID (if not 0)
	ToID   int    // Filter by target node ID (if not 0)
}

// GetEdge retrieves an edge by its ID from the graph database.
func (c *client) GetEdge(id int) (*GraphEdge, error) {
	var edge *GraphEdge
	if err := c.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(edgeBucket))
		if bucket == nil {
			return fmt.Errorf("edge bucket not found")
		}

		data := bucket.Get(itob(id))
		if data == nil {
			return fmt.Errorf("edge with ID %d not found", id)
		}

		edge = &GraphEdge{}
		if err := json.Unmarshal(data, edge); err != nil {
			return fmt.Errorf("failed to unmarshal edge: %w", err)
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to get edge: %w", err)
	}

	return edge, nil
}

// ListEdges retrieves all edges from the graph database.
func (c *client) ListEdges(filter EdgeFilter) ([]GraphEdge, error) {
	var edges []GraphEdge
	if err := c.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(edgeBucket))
		if bucket == nil {
			return fmt.Errorf("edge bucket not found")
		}

		cursor := bucket.Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			var edge GraphEdge
			if err := json.Unmarshal(v, &edge); err != nil {
				return fmt.Errorf("failed to unmarshal edge: %w", err)
			}

			// Apply filters if specified
			if filter.Type != "" && edge.Type != filter.Type {
				continue
			}
			if filter.FromID != 0 && edge.FromID != filter.FromID {
				continue
			}
			if filter.ToID != 0 && edge.ToID != filter.ToID {
				continue
			}

			edges = append(edges, edge)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to list edges: %w", err)
	}
	return edges, nil
}

// CreateEdge adds a new edge to the graph database.
func (c *client) CreateEdge(edgeType string, fromID, toID int) (*GraphEdge, error) {
	var edge *GraphEdge
	if err := c.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(edgeBucket))
		if bucket == nil {
			return fmt.Errorf("edge bucket not found")
		}

		// Check if the nodes exist
		nodeBucket := tx.Bucket([]byte(nodeBucket))
		if nodeBucket == nil {
			return fmt.Errorf("node bucket not found")
		}

		if nodeBucket.Get(itob(fromID)) == nil {
			return fmt.Errorf("source node not found")
		}

		if nodeBucket.Get(itob(toID)) == nil {
			return fmt.Errorf("target node not found")
		}

		// Get next sequence for edge ID
		id, err := bucket.NextSequence()
		if err != nil {
			return fmt.Errorf("failed to get next sequence: %w", err)
		}

		// Create the edge
		edge = &GraphEdge{
			ID:     int(id),
			Type:   edgeType,
			FromID: fromID,
			ToID:   toID,
		}

		// Marshal the edge
		data, err := json.Marshal(edge)
		if err != nil {
			return fmt.Errorf("failed to marshal edge: %w", err)
		}

		// Store it in the database
		if err := bucket.Put(edge.BID(), data); err != nil {
			return fmt.Errorf("failed to put edge into db: %w", err)
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to create edge: %w", err)
	}

	return edge, nil
}

// DeleteEdge removes an edge from the graph database.
func (c *client) DeleteEdge(id int) error {
	if err := c.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(edgeBucket))
		if bucket == nil {
			return fmt.Errorf("edge bucket not found")
		}

		if err := bucket.Delete(itob(id)); err != nil {
			return fmt.Errorf("failed to delete edge from db: %w", err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("failed to delete edge: %w", err)
	}

	return nil
}

func itob(i int) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(i))
	return b
}

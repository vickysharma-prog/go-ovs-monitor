// Package ovsdb is a small client for the Open vSwitch Database management
// protocol (OVSDB, RFC 7047). It speaks JSON-RPC directly over the ovsdb-server
// socket rather than shelling out to ovs-vsctl, so the tool talks to OVS the
// same way OVN and ovn-kubernetes do.
package ovsdb

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Client is a JSON-RPC connection to ovsdb-server.
type Client struct {
	conn   net.Conn
	dec    *json.Decoder
	enc    *json.Encoder
	nextID int
}

type rpcRequest struct {
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
	ID     int           `json:"id"`
}

// rpcMessage covers both replies (Result/Error/ID set) and server-initiated
// notifications such as "echo" keepalives and "update" monitor events (Method
// and Params set, ID null).
type rpcMessage struct {
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// Dial connects to an ovsdb-server endpoint. target is "unix:/path/to.sock" or
// "tcp:host:port".
func Dial(target string) (*Client, error) {
	network, addr, err := parseTarget(target)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialTimeout(network, addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", target, err)
	}
	return &Client{
		conn:   conn,
		dec:    json.NewDecoder(conn),
		enc:    json.NewEncoder(conn),
		nextID: 1,
	}, nil
}

func parseTarget(target string) (network, addr string, err error) {
	switch {
	case strings.HasPrefix(target, "unix:"):
		return "unix", strings.TrimPrefix(target, "unix:"), nil
	case strings.HasPrefix(target, "tcp:"):
		return "tcp", strings.TrimPrefix(target, "tcp:"), nil
	default:
		return "", "", fmt.Errorf("unsupported db target %q (use unix:/path or tcp:host:port)", target)
	}
}

// Close releases the connection.
func (c *Client) Close() error { return c.conn.Close() }

// Call sends a JSON-RPC request and returns its result. It transparently replies
// to any "echo" keepalive the server sends while we wait for our answer.
func (c *Client) Call(method string, params ...interface{}) (json.RawMessage, error) {
	id := c.nextID
	c.nextID++
	if params == nil {
		params = []interface{}{}
	}
	if err := c.enc.Encode(rpcRequest{Method: method, Params: params, ID: id}); err != nil {
		return nil, err
	}
	for {
		var msg rpcMessage
		if err := c.dec.Decode(&msg); err != nil {
			return nil, err
		}
		if msg.Method == "echo" {
			c.replyEcho(msg)
			continue
		}
		if string(msg.ID) == strconv.Itoa(id) {
			if len(msg.Error) > 0 && string(msg.Error) != "null" {
				return nil, fmt.Errorf("ovsdb error: %s", msg.Error)
			}
			return msg.Result, nil
		}
		// A notification for something else; keep reading for our reply.
	}
}

// Monitor registers a monitor request on the given tables and returns the
// server's initial snapshot. Call NextUpdate in a loop afterwards to receive
// live changes.
func (c *Client) Monitor(db string, tables map[string]interface{}) (json.RawMessage, error) {
	return c.Call("monitor", db, "ovs-monitor", tables)
}

// NextUpdate blocks until the next "update" notification arrives and returns its
// params. Echo keepalives are answered automatically.
func (c *Client) NextUpdate() (json.RawMessage, error) {
	for {
		var msg rpcMessage
		if err := c.dec.Decode(&msg); err != nil {
			return nil, err
		}
		switch msg.Method {
		case "echo":
			c.replyEcho(msg)
		case "update":
			return msg.Params, nil
		}
	}
}

func (c *Client) replyEcho(msg rpcMessage) {
	// An echo reply must carry the same id and echo the params back.
	_ = c.enc.Encode(struct {
		Result json.RawMessage `json:"result"`
		Error  interface{}     `json:"error"`
		ID     json.RawMessage `json:"id"`
	}{Result: msg.Params, Error: nil, ID: msg.ID})
}

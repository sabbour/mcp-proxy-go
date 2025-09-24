package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Client is a minimal JSON-RPC client built on top of a Transport.
type Client struct {
	transport Transport
	requests  sync.Map
	onClose   func()
	seq       atomic.Uint64
}

// NewClient creates a client bound to the provided transport.
func NewClient(transport Transport) *Client {
	c := &Client{transport: transport}

	transport.OnMessage(func(msg Message) {
		var resp Response
		if err := json.Unmarshal(msg.Bytes(), &resp); err != nil {
			return
		}

		if len(resp.ID) == 0 {
			return
		}

		if ch, ok := c.requests.LoadAndDelete(string(resp.ID)); ok {
			ch.(chan Message) <- msg
		}
	})

	transport.OnClose(func() {
		if c.onClose != nil {
			c.onClose()
		}
	})

	return c
}

// Start starts the underlying transport.
func (c *Client) Start(ctx context.Context) error {
	return c.transport.Start(ctx)
}

// Close shuts down the underlying transport.
func (c *Client) Close() error {
	return c.transport.Close()
}

// Notify sends a notification to the remote server.
func (c *Client) Notify(ctx context.Context, method string, params any) error {
	payload := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		payload["params"] = params
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return c.transport.Send(ctx, NewMessage(raw))
}

// Call sends a request to the remote server and returns the message response.
func (c *Client) Call(ctx context.Context, method string, params any) (Message, error) {
	id := c.seq.Add(1)

	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		payload["params"] = params
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return Message{}, err
	}

	idBytes, err := json.Marshal(id)
	if err != nil {
		return Message{}, err
	}
	idKey := string(idBytes)
	ch := make(chan Message, 1)
	c.requests.Store(idKey, ch)

	if err := c.transport.Send(ctx, NewMessage(raw)); err != nil {
		c.requests.Delete(idKey)
		return Message{}, err
	}

	select {
	case <-ctx.Done():
		c.requests.Delete(idKey)
		return Message{}, ctx.Err()
	case msg := <-ch:
		return msg, nil
	}
}

// OnClose registers a callback invoked when the underlying transport closes.
func (c *Client) OnClose(f func()) {
	c.onClose = f
}

// AwaitResult decodes a JSON-RPC response message into target.
func AwaitResult(msg Message, target any) error {
	var resp Response
	if err := json.Unmarshal(msg.Bytes(), &resp); err != nil {
		return err
	}

	if resp.Error != nil {
		return errors.New(resp.Error.Message)
	}

	if target == nil {
		return nil
	}

	if len(resp.Result) == 0 {
		return errors.New("empty result payload")
	}

	return json.Unmarshal(resp.Result, target)
}

// BlockingCall is a helper that performs Call with a timeout.
func (c *Client) BlockingCall(ctx context.Context, timeout time.Duration, method string, params any, target any) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	msg, err := c.Call(ctx, method, params)
	if err != nil {
		return err
	}

	return AwaitResult(msg, target)
}

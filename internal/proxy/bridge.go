package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/sabbour/mcp-proxy-go/internal/mcp"
)

// Bridge forwards JSON-RPC messages between two transports while namespacing IDs to avoid collisions.
type Bridge struct {
	left        mcp.Transport
	right       mcp.Transport
	startedOnce sync.Once

	leftMap  sync.Map
	rightMap sync.Map

	leftSeq  atomic.Uint64
	rightSeq atomic.Uint64
}

// NewBridge creates a new bridge between two transports.
func NewBridge(left, right mcp.Transport) *Bridge {
	b := &Bridge{left: left, right: right}
	left.OnMessage(b.onLeftMessage)
	right.OnMessage(b.onRightMessage)

	left.OnError(func(err error) { right.Send(context.Background(), mcp.NewMessage(buildErrorNotification("left", err))) })
	right.OnError(func(err error) { left.Send(context.Background(), mcp.NewMessage(buildErrorNotification("right", err))) })

	left.OnClose(func() { right.Close() })
	right.OnClose(func() { left.Close() })

	return b
}

func (b *Bridge) Start(ctx context.Context) error {
	var errLeft, errRight error
	b.startedOnce.Do(func() {
		errLeft = b.left.Start(ctx)
		if errLeft != nil {
			return
		}
		errRight = b.right.Start(ctx)
		if errRight != nil {
			_ = b.left.Close()
		}
	})

	if errLeft != nil {
		return errLeft
	}
	return errRight
}

func (b *Bridge) Close() error {
	_ = b.left.Close()
	_ = b.right.Close()
	return nil
}

func (b *Bridge) onLeftMessage(msg mcp.Message) {
	b.forward(msg, b.left, b.right, &b.leftSeq, &b.leftMap, &b.rightMap)
}

func (b *Bridge) onRightMessage(msg mcp.Message) {
	b.forward(msg, b.right, b.left, &b.rightSeq, &b.rightMap, &b.leftMap)
}

func (b *Bridge) forward(msg mcp.Message, from mcp.Transport, to mcp.Transport, seq *atomic.Uint64, requestMap *sync.Map, responseMap *sync.Map) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(msg.Bytes(), &envelope); err != nil {
		// Non-JSON payload, forward as-is.
		_ = to.Send(context.Background(), msg)
		return
	}

	if _, ok := envelope["id"]; !ok {
		_ = to.Send(context.Background(), msg)
		return
	}

	if _, ok := envelope["method"]; ok {
		// Request - namespace ID
		origID := envelope["id"]
		proxyID := fmt.Sprintf("proxy-%d", seq.Add(1))
		envelope["id"] = json.RawMessage(strQuote(proxyID))
		requestMap.Store(proxyID, origID)

		raw, err := json.Marshal(envelope)
		if err != nil {
			requestMap.Delete(proxyID)
			return
		}

		_ = to.Send(context.Background(), mcp.NewMessage(raw))
		return
	}

	// Response - translate back.
	var id string
	if err := json.Unmarshal(envelope["id"], &id); err != nil {
		// Unknown format, forward unchanged
		_ = to.Send(context.Background(), msg)
		return
	}

	if orig, ok := responseMap.Load(id); ok {
		envelope["id"] = orig.(json.RawMessage)
		raw, err := json.Marshal(envelope)
		if err == nil {
			_ = to.Send(context.Background(), mcp.NewMessage(raw))
			responseMap.Delete(id)
			return
		}
	}

	_ = to.Send(context.Background(), msg)
}

func buildErrorNotification(source string, err error) []byte {
	payload := map[string]any{
		"jsonrpc": "2.0",
		"method":  "mcp-proxy/error",
		"params": map[string]any{
			"source": source,
			"error":  err.Error(),
		},
	}

	raw, _ := json.Marshal(payload)
	return raw
}

func strQuote(s string) []byte {
	return []byte(fmt.Sprintf("\"%s\"", s))
}

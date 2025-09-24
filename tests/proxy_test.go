package tests

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sabbour/mcp-proxy-go/internal/mcp"
	"github.com/sabbour/mcp-proxy-go/internal/proxy"
)

// mockTransport implements mcp.Transport for testing
type mockTransport struct {
	messages []mcp.Message
	onMsg    func(mcp.Message)
	onErr    func(error)
	onClose  func()
	mu       sync.RWMutex
	closed   bool
}

func newMockTransport() *mockTransport {
	return &mockTransport{}
}

func (m *mockTransport) Start(ctx context.Context) error {
	return nil
}

func (m *mockTransport) Send(ctx context.Context, msg mcp.Message) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if m.closed {
		return nil
	}
	
	m.messages = append(m.messages, msg)
	return nil
}

func (m *mockTransport) OnMessage(handler func(mcp.Message)) {
	m.onMsg = handler
}

func (m *mockTransport) OnError(handler func(error)) {
	m.onErr = handler
}

func (m *mockTransport) OnClose(handler func()) {
	m.onClose = handler
}

func (m *mockTransport) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.closed {
		return nil
	}
	
	m.closed = true
	if m.onClose != nil {
		go m.onClose()
	}
	return nil
}

func (m *mockTransport) getMessages() []mcp.Message {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// Return a copy to avoid race conditions
	messages := make([]mcp.Message, len(m.messages))
	copy(messages, m.messages)
	return messages
}

func (m *mockTransport) simulateMessage(msg mcp.Message) {
	if m.onMsg != nil {
		m.onMsg(msg)
	}
}

func TestBridge(t *testing.T) {
	t.Run("forwards JSON-RPC request with ID namespace", func(t *testing.T) {
		left := newMockTransport()
		right := newMockTransport()
		
		bridge := proxy.NewBridge(left, right)
		require.NotNil(t, bridge)
		
		// Start the bridge
		err := bridge.Start(context.Background())
		require.NoError(t, err)
		
		// Simulate a JSON-RPC request from left to right
		request := map[string]any{
			"jsonrpc": "2.0",
			"method":  "initialize",
			"id":      "test-123",
		}
		requestBytes, _ := json.Marshal(request)
		
		left.simulateMessage(mcp.NewMessage(requestBytes))
		
		// Give it a moment to process
		time.Sleep(10 * time.Millisecond)
		
		// Check that right received a message with namespaced ID
		messages := right.getMessages()
		require.Len(t, messages, 1)
		
		var received map[string]any
		err = json.Unmarshal(messages[0].Bytes(), &received)
		require.NoError(t, err)
		
		require.Equal(t, "2.0", received["jsonrpc"])
		require.Equal(t, "initialize", received["method"])
		require.Contains(t, received["id"], "proxy-")
		require.NotEqual(t, "test-123", received["id"])
	})
	
	t.Run("forwards non-JSON messages unchanged", func(t *testing.T) {
		left := newMockTransport()
		right := newMockTransport()
		
		bridge := proxy.NewBridge(left, right)
		err := bridge.Start(context.Background())
		require.NoError(t, err)
		
		// Send non-JSON data
		nonJSON := []byte("This is not JSON")
		left.simulateMessage(mcp.NewMessage(nonJSON))
		
		time.Sleep(10 * time.Millisecond)
		
		// Should be forwarded unchanged
		messages := right.getMessages()
		require.Len(t, messages, 1)
		require.Equal(t, nonJSON, messages[0].Bytes())
	})
	
	t.Run("forwards notifications unchanged", func(t *testing.T) {
		left := newMockTransport()
		right := newMockTransport()
		
		bridge := proxy.NewBridge(left, right)
		err := bridge.Start(context.Background())
		require.NoError(t, err)
		
		// Send notification (no ID)
		notification := map[string]any{
			"jsonrpc": "2.0",
			"method":  "notification",
		}
		notificationBytes, _ := json.Marshal(notification)
		
		left.simulateMessage(mcp.NewMessage(notificationBytes))
		
		time.Sleep(10 * time.Millisecond)
		
		// Should be forwarded unchanged
		messages := right.getMessages()
		require.Len(t, messages, 1)
		require.Equal(t, notificationBytes, messages[0].Bytes())
	})
	
	t.Run("can be closed cleanly", func(t *testing.T) {
		left := newMockTransport()
		right := newMockTransport()
		
		bridge := proxy.NewBridge(left, right)
		err := bridge.Start(context.Background())
		require.NoError(t, err)
		
		err = bridge.Close()
		require.NoError(t, err)
		
		// Both transports should be closed
		require.True(t, left.closed)
		require.True(t, right.closed)
	})
}
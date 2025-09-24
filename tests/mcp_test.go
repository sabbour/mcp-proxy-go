package tests

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sabbour/mcp-proxy-go/internal/mcp"
)

func TestMessage(t *testing.T) {
	t.Run("creates and marshals message", func(t *testing.T) {
		originalJSON := `{"jsonrpc": "2.0", "id": 1, "method": "test"}`
		msg := mcp.NewMessage([]byte(originalJSON))
		
		marshaled, err := json.Marshal(msg)
		require.NoError(t, err)
		require.JSONEq(t, originalJSON, string(marshaled))
	})

	t.Run("preserves original bytes", func(t *testing.T) {
		originalJSON := `{"jsonrpc": "2.0", "id": 1, "method": "test"}`
		msg := mcp.NewMessage([]byte(originalJSON))
		
		require.Equal(t, []byte(originalJSON), msg.Bytes())
	})

	t.Run("unmarshals from JSON", func(t *testing.T) {
		originalJSON := `{"jsonrpc": "2.0", "id": 1, "method": "test"}`
		
		var msg mcp.Message
		err := json.Unmarshal([]byte(originalJSON), &msg)
		require.NoError(t, err)
		require.Equal(t, []byte(originalJSON), msg.Bytes())
	})
}

func TestIsInitializeRequest(t *testing.T) {
	t.Run("identifies initialize request", func(t *testing.T) {
		initJSON := `{"jsonrpc": "2.0", "id": 1, "method": "initialize"}`
		require.True(t, mcp.IsInitializeRequest([]byte(initJSON)))
	})

	t.Run("rejects non-initialize request", func(t *testing.T) {
		otherJSON := `{"jsonrpc": "2.0", "id": 1, "method": "other"}`
		require.False(t, mcp.IsInitializeRequest([]byte(otherJSON)))
	})

	t.Run("rejects invalid JSON", func(t *testing.T) {
		invalidJSON := `{"invalid": json`
		require.False(t, mcp.IsInitializeRequest([]byte(invalidJSON)))
	})

	t.Run("rejects non-2.0 jsonrpc", func(t *testing.T) {
		oldVersion := `{"jsonrpc": "1.0", "id": 1, "method": "initialize"}`
		require.False(t, mcp.IsInitializeRequest([]byte(oldVersion)))
	})
}

func TestIsNotification(t *testing.T) {
	t.Run("identifies notification without id", func(t *testing.T) {
		notification := `{"jsonrpc": "2.0", "method": "notify"}`
		require.True(t, mcp.IsNotification([]byte(notification)))
	})

	t.Run("rejects request with id", func(t *testing.T) {
		request := `{"jsonrpc": "2.0", "id": 1, "method": "request"}`
		require.False(t, mcp.IsNotification([]byte(request)))
	})

	t.Run("rejects invalid JSON", func(t *testing.T) {
		invalidJSON := `{"invalid": json`
		require.False(t, mcp.IsNotification([]byte(invalidJSON)))
	})
}
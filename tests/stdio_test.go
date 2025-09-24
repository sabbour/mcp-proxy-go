package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sabbour/mcp-proxy-go/internal/mcp"
	"github.com/sabbour/mcp-proxy-go/internal/stdio"
)

func TestStdioClientParams(t *testing.T) {
	t.Run("creates client with valid params", func(t *testing.T) {
		params := stdio.Params{
			Command: "echo",
			Args:    []string{"hello"},
			Dir:     "/tmp",
			Env:     []string{"TEST=1"},
		}

		client := stdio.NewClient(params)
		require.NotNil(t, client)
		
		// Test that callbacks can be set without panicking
		client.OnMessage(func(msg mcp.Message) {})
		client.OnError(func(err error) {})
		client.OnClose(func() {})
	})
}

func TestStdioClientBasicLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stdio integration test in short mode")
	}

	t.Run("can start and close echo command", func(t *testing.T) {
		params := stdio.Params{
			Command: "echo",
			Args:    []string{"test"},
		}

		client := stdio.NewClient(params)
		require.NotNil(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := client.Start(ctx)
		require.NoError(t, err)

		// Give it a moment to run
		time.Sleep(100 * time.Millisecond)

		err = client.Close()
		require.NoError(t, err)
	})
}
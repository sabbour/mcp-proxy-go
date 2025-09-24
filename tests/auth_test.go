package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sabbour/mcp-proxy-go/internal/auth"
)

func TestAuthMiddleware(t *testing.T) {
	t.Run("no auth configured allows all requests", func(t *testing.T) {
		middleware := auth.New(auth.Config{})
		req, _ := http.NewRequest("GET", "/test", nil)
		
		require.True(t, middleware.Validate(req))
	})

	t.Run("valid API key is accepted", func(t *testing.T) {
		apiKey := "test-key-123"
		middleware := auth.New(auth.Config{APIKey: apiKey})
		req, _ := http.NewRequest("GET", "/test", nil)
		req.Header.Set("X-API-Key", apiKey)
		
		require.True(t, middleware.Validate(req))
	})

	t.Run("missing API key is rejected", func(t *testing.T) {
		middleware := auth.New(auth.Config{APIKey: "test-key"})
		req, _ := http.NewRequest("GET", "/test", nil)
		
		require.False(t, middleware.Validate(req))
	})

	t.Run("wrong API key is rejected", func(t *testing.T) {
		middleware := auth.New(auth.Config{APIKey: "correct-key"})
		req, _ := http.NewRequest("GET", "/test", nil)
		req.Header.Set("X-API-Key", "wrong-key")
		
		require.False(t, middleware.Validate(req))
	})

	t.Run("unauthorized response format", func(t *testing.T) {
		middleware := auth.New(auth.Config{APIKey: "test"})
		code, headers, body := middleware.UnauthorizedResponse()
		
		require.Equal(t, http.StatusUnauthorized, code)
		require.Equal(t, "application/json", headers.Get("Content-Type"))
		require.Contains(t, string(body), "Unauthorized")
		require.Contains(t, string(body), "jsonrpc")
	})
}
package tests

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sabbour/mcp-proxy-go/internal/eventstore"
	"github.com/sabbour/mcp-proxy-go/internal/httpserver"
	"github.com/sabbour/mcp-proxy-go/internal/mcp"
	"github.com/sabbour/mcp-proxy-go/internal/stdio"
)

func TestHTTPProxyStream(t *testing.T) {
	server, baseURL := startTestServer(t, httpserver.Options{})
	t.Cleanup(func() {
		require.NoError(t, server.Close(context.Background()))
	})

	sessionID := initializeSession(t, baseURL, "")

	resp := postJSON(t, baseURL+"/mcp", sessionID, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "resources/list",
	})

	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]any
	decodeBody(t, resp.Body, &body)

	result := body["result"].(map[string]any)
	resources := result["resources"].([]any)
	require.Len(t, resources, 1)
	resource := resources[0].(map[string]any)
	require.Equal(t, "Example Resource", resource["name"])
	require.Equal(t, "file:///example.txt", resource["uri"])
}

func TestHTTPProxyAuth(t *testing.T) {
	apiKey := "secret"
	server, baseURL := startTestServer(t, httpserver.Options{APIKey: apiKey})
	t.Cleanup(func() {
		require.NoError(t, server.Close(context.Background()))
	})

	reqBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
	}

	// Missing API key
	reqBytes, _ := json.Marshal(reqBody)
	resp, err := http.Post(baseURL+"/mcp", "application/json", bytes.NewReader(reqBytes))
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// With API key
	req, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", bytes.NewReader(reqBytes))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotEmpty(t, resp.Header.Get("mcp-session-id"))
}

func TestHTTPProxyStateless(t *testing.T) {
	server, baseURL := startTestServer(t, httpserver.Options{Stateless: true})
	t.Cleanup(func() {
		require.NoError(t, server.Close(context.Background()))
	})

	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "resources/list",
	}

	reqBytes, _ := json.Marshal(req)

	reqInitialize := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
	}

	reqInitBytes, _ := json.Marshal(reqInitialize)

	resp, err := http.Post(baseURL+"/mcp", "application/json", bytes.NewReader(reqInitBytes))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	req2, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", bytes.NewReader(reqBytes))
	require.NoError(t, err)
	req2.Header.Set("Content-Type", "application/json")

	resp, err = http.DefaultClient.Do(req2)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHTTPProxySSE(t *testing.T) {
	server, baseURL := startTestServer(t, httpserver.Options{})
	t.Cleanup(func() {
		require.NoError(t, server.Close(context.Background()))
	})

	sessionID := initializeSession(t, baseURL, "")

	req, err := http.NewRequest(http.MethodGet, baseURL+"/sse", nil)
	require.NoError(t, err)
	req.Header.Set("mcp-session-id", sessionID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close() // Ensure body is closed
	require.Equal(t, http.StatusOK, resp.StatusCode)

	reader := bufio.NewReader(resp.Body)

	// Use a channel to coordinate the test
	done := make(chan bool, 1)
	
	go func() {
		defer func() { done <- true }()
		postJSON(t, baseURL+"/mcp", sessionID, map[string]any{
			"jsonrpc": "2.0",
			"id":      5,
			"method":  "resources/list",
		})
	}()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		// Use a shorter read timeout to avoid hanging
		select {
		case <-time.After(100 * time.Millisecond):
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				t.Logf("Error reading SSE: %v", err)
				continue
			}

			if strings.HasPrefix(line, "data:") {
				payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if strings.Contains(payload, "resources") {
					// Wait for the goroutine to complete
					<-done
					return
				}
			}
		}
	}

	t.Fatalf("did not receive SSE event in time")
}

func startTestServer(t *testing.T, opts httpserver.Options) (*httpserver.Server, string) {
	t.Helper()

	host := "127.0.0.1"
	port := freePort(t)

	root := projectRoot(t)

	opts.Host = host
	opts.Port = port
	opts.EventStoreFactory = func() *eventstore.Memory {
		return eventstore.NewMemory()
	}
	opts.CreateTransport = func(ctx context.Context, _ *http.Request) (mcp.Transport, error) {
		params := stdio.Params{
			Command: "go",
			Args:    []string{"run", "./fixtures/simple_stdio_server.go"},
			Dir:     root,
		}
		return stdio.NewClient(params), nil
	}

	srv, err := httpserver.Start(opts)
	require.NoError(t, err)

	baseURL := fmt.Sprintf("http://%s:%d", host, port)
	waitForServer(t, baseURL)

	return srv, baseURL
}

func waitForServer(t *testing.T, baseURL string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/ping")
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			return
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("server did not become ready")
}

func freePort(t *testing.T) int {
	tl, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer tl.Close()

	return tl.Addr().(*net.TCPAddr).Port
}

func projectRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Join(wd, "..")
}

func initializeSession(t *testing.T, baseURL, apiKey string) string {
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
	}

	resp := postJSON(t, baseURL+"/mcp", "", req, header("X-API-Key", apiKey))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	sessionID := resp.Header.Get("mcp-session-id")
	require.NotEmpty(t, sessionID)
	return sessionID
}

func postJSON(t *testing.T, url, sessionID string, payload any, headers ...func(*http.Request)) *http.Response {
	reqBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(reqBytes))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		req.Header.Set("mcp-session-id", sessionID)
	}

	for _, apply := range headers {
		if apply != nil {
			apply(req)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func decodeBody(t *testing.T, r io.ReadCloser, v any) {
	t.Helper()
	defer r.Close()
	require.NoError(t, json.NewDecoder(r).Decode(v))
}

func header(name, value string) func(*http.Request) {
	return func(req *http.Request) {
		if value != "" {
			req.Header.Set(name, value)
		}
	}
}
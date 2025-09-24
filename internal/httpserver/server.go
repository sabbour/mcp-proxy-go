// HTTP server implementation for MCP Proxy
//
// This package provides HTTP and SSE transport capabilities adapted from
// the original TypeScript mcp-proxy implementation:
// https://github.com/punkpeye/mcp-proxy
//
// The server routing, session management, and authentication patterns
// follow the design established in the original project.
package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/sabbour/mcp-proxy-go/internal/auth"
	"github.com/sabbour/mcp-proxy-go/internal/eventstore"
	"github.com/sabbour/mcp-proxy-go/internal/mcp"
)

// generateSessionID creates a new unique session ID
func generateSessionID() string {
	return uuid.New().String()
}

// Options configure the HTTP proxy server.
type Options struct {
	Host               string
	Port               int
	APIKey             string
	CreateTransport    func(ctx context.Context, r *http.Request) (mcp.Transport, error)
	EventStoreFactory  func() *eventstore.Memory
	StreamEndpoint     string
	SSEEndpoint        string
	Stateless          bool
	EnableJSONResponse bool
	OnConnect          func(sessionID string)
	OnClose            func(sessionID string)
	OnUnhandled        func(http.ResponseWriter, *http.Request)
}

// Server represents the running HTTP proxy.
type Server struct {
	server   *http.Server
	opts     Options
	auth     *auth.Middleware
	sessions sync.Map // sessionID -> *session
}

// Start creates and runs the HTTP server.
func Start(opts Options) (*Server, error) {
	if opts.StreamEndpoint == "" {
		opts.StreamEndpoint = "/mcp"
	}
	if opts.SSEEndpoint == "" {
		opts.SSEEndpoint = "/sse"
	}

	authMiddleware := auth.New(auth.Config{APIKey: opts.APIKey})

	s := &Server{opts: opts, auth: authMiddleware}

	httpServer := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", opts.Host, opts.Port),
		Handler: http.HandlerFunc(s.handle),
	}

	s.server = httpServer

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[mcp-proxy] http server error: %v", err)
		}
	}()

	// Wait briefly for the server to bind.
	time.Sleep(50 * time.Millisecond)

	return s, nil
}

// Close gracefully shuts down the server.
func (s *Server) Close(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	log.Printf("[mcp-proxy] DEBUG: Incoming request: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
	
	// Set CORS headers
	origin := r.Header.Get("Origin")
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, mcp-session-id")
	w.Header().Set("Access-Control-Expose-Headers", "mcp-session-id")

	if r.Method == http.MethodOptions {
		log.Printf("[mcp-proxy] DEBUG: Handling OPTIONS request")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.URL.Path == "/ping" && r.Method == http.MethodGet {
		log.Printf("[mcp-proxy] DEBUG: Handling ping request")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("pong"))
		return
	}

	log.Printf("[mcp-proxy] DEBUG: Validating authentication")
	if !s.auth.Validate(r) {
		log.Printf("[mcp-proxy] DEBUG: Authentication failed")
		code, headers, body := s.auth.UnauthorizedResponse()
		for k, vals := range headers {
			for _, v := range vals {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(code)
		_, _ = w.Write(body)
		return
	}
	log.Printf("[mcp-proxy] DEBUG: Authentication passed")

	switch {
	case r.URL.Path == s.opts.StreamEndpoint:
		log.Printf("[mcp-proxy] DEBUG: Routing to stream endpoint (%s) with method %s", s.opts.StreamEndpoint, r.Method)
		s.handleStream(w, r)
	case r.URL.Path == s.opts.SSEEndpoint:
		log.Printf("[mcp-proxy] DEBUG: Routing to SSE endpoint (%s)", s.opts.SSEEndpoint)
		s.handleSSE(w, r)
	default:
		log.Printf("[mcp-proxy] DEBUG: No matching endpoint for %s, available: %s, %s", r.URL.Path, s.opts.StreamEndpoint, s.opts.SSEEndpoint)
		if s.opts.OnUnhandled != nil {
			s.opts.OnUnhandled(w, r)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	log.Printf("[mcp-proxy] DEBUG: handleStream called with method %s", r.Method)
	
	if r.Method == http.MethodDelete {
		log.Printf("[mcp-proxy] DEBUG: Handling DELETE request")
		s.handleDelete(w, r)
		return
	}

	if r.Method != http.MethodPost {
		log.Printf("[mcp-proxy] DEBUG: Method not allowed: %s", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	log.Printf("[mcp-proxy] DEBUG: Reading request body")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[mcp-proxy] DEBUG: Error reading request body: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	log.Printf("[mcp-proxy] DEBUG: Request body: %s", string(body))

	// Debug: Log all headers to see what's being sent
	log.Printf("[mcp-proxy] DEBUG: Request headers:")
	for name, values := range r.Header {
		for _, value := range values {
			log.Printf("[mcp-proxy] DEBUG:   %s: %s", name, value)
		}
	}

	sessionID := r.Header.Get("mcp-session-id")
	log.Printf("[mcp-proxy] DEBUG: Session ID from header: '%s'", sessionID)

	if sessionID == "" {
		log.Printf("[mcp-proxy] DEBUG: No session ID, checking if initialize request")
		if !mcp.IsInitializeRequest(body) && !s.opts.Stateless {
			log.Printf("[mcp-proxy] DEBUG: Not initialize request and not stateless - returning bad request")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("missing session id"))
			return
		}

		sess, newID, err := s.createSession(r.Context(), r)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			return
		}

		if !s.opts.Stateless {
			w.Header().Set("mcp-session-id", newID)
			log.Printf("[mcp-proxy] DEBUG: Set session ID header in response: '%s'", newID)
		}

		resp, err := sess.request(r.Context(), body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			return
		}

		if resp != nil {
			s.writeJSONResponse(w, resp)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}

		if s.opts.Stateless {
			_ = sess.close()
		}

		return
	}

	sessAny, ok := s.sessions.Load(sessionID)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("session not found"))
		return
	}

	sess := sessAny.(*session)

	resp, err := sess.request(r.Context(), body)
	if err != nil {
		log.Printf("[mcp-proxy] DEBUG: Session request error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	if resp != nil {
		log.Printf("[mcp-proxy] DEBUG: Sending JSON response: %s", string(resp))
		s.writeJSONResponse(w, resp)
	} else {
		log.Printf("[mcp-proxy] DEBUG: No response from stdio server - returning 204 (normal for notifications)")
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("mcp-session-id")
	if sessionID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("missing session id"))
		return
	}

	sessAny, ok := s.sessions.Load(sessionID)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	sess := sessAny.(*session)
	_ = sess.close()

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	log.Printf("[mcp-proxy] DEBUG: handleSSE called with method %s, URL path: %s", r.Method, r.URL.Path)
	
	if r.Method != http.MethodGet {
		log.Printf("[mcp-proxy] DEBUG: SSE handler only accepts GET requests, got %s", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	log.Printf("[mcp-proxy] DEBUG: Creating SSE transport for endpoint %s", s.opts.SSEEndpoint)
	
	// Set SSE headers immediately
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	
	// Create session ID
	sessionID := generateSessionID()
	log.Printf("[mcp-proxy] DEBUG: Generated session ID: %s", sessionID)
	
		// Create server using the callback
	transport, err := s.opts.CreateTransport(r.Context(), r)
	if err != nil {
		log.Printf("[mcp-proxy] DEBUG: Error creating MCP transport: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Error creating server"))
		return
	}
	
	log.Printf("[mcp-proxy] DEBUG: Created MCP transport successfully")
	
	// Use the transport (simplified for now)
	_ = transport
	
	// Set up event stream
	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Printf("[mcp-proxy] DEBUG: ResponseWriter doesn't support flushing")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Write 200 OK status
	w.WriteHeader(http.StatusOK)
	
	// Send initial connection message matching TypeScript format
	initialMsg := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "sse/connection",
		"params": map[string]interface{}{
			"message": "SSE Connection established",
		},
	}
	
	msgBytes, _ := json.Marshal(initialMsg)
	fmt.Fprintf(w, "data: %s\n\n", msgBytes)
	flusher.Flush()
	
	log.Printf("[mcp-proxy] DEBUG: Sent initial SSE connection message")
	
	// Keep connection alive
	ctx := r.Context()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			log.Printf("[mcp-proxy] DEBUG: SSE connection closed by client")
			return
		case <-ticker.C:
			// Send keepalive
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) createSession(ctx context.Context, r *http.Request) (*session, string, error) {
	if s.opts.CreateTransport == nil {
		return nil, "", fmt.Errorf("CreateTransport not configured")
	}

	transport, err := s.opts.CreateTransport(ctx, r)
	if err != nil {
		return nil, "", err
	}

	sessionID := uuid.NewString()
	store := s.opts.EventStoreFactory
	var mem *eventstore.Memory
	if store != nil && !s.opts.Stateless {
		mem = store()
	}

	finalize := func() {
		if !s.opts.Stateless {
			s.sessions.Delete(sessionID)
		}
		if s.opts.OnClose != nil {
			s.opts.OnClose(sessionID)
		}
	}

	sess := newSession(sessionID, transport, mem, finalize)

	if err := sess.start(context.Background()); err != nil {
		return nil, "", err
	}

	if !s.opts.Stateless {
		s.sessions.Store(sessionID, sess)
	}

	if s.opts.OnConnect != nil {
		s.opts.OnConnect(sessionID)
	}

	return sess, sessionID, nil
}

func (s *Server) writeJSONResponse(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func writeSSE(w http.ResponseWriter, ev eventstore.Event) {
	if ev.ID != "" {
		fmt.Fprintf(w, "id: %s\n", ev.ID)
	}
	fmt.Fprintf(w, "data: %s\n\n", ev.Payload)
}

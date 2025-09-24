package auth

import (
	"encoding/json"
	"net/http"
)

// Config configures the authentication middleware.
type Config struct {
	APIKey string
}

// Middleware validates requests using the configured API key.
type Middleware struct {
	cfg Config
}

// New creates a new Middleware instance.
func New(cfg Config) *Middleware {
	return &Middleware{cfg: cfg}
}

// Validate determines whether the HTTP request is authorized.
func (m *Middleware) Validate(r *http.Request) bool {
	if m.cfg.APIKey == "" {
		return true
	}

	key := r.Header.Get("X-API-Key")
	return key == m.cfg.APIKey
}

// UnauthorizedResponse returns the appropriate HTTP status and JSON-RPC error response for unauthorized requests.
func (m *Middleware) UnauthorizedResponse() (int, http.Header, []byte) {
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	
	response := map[string]any{
		"error": map[string]any{
			"code":    401,
			"message": "Unauthorized: Invalid or missing API key",
		},
		"id":      nil,
		"jsonrpc": "2.0",
	}
	
	body, _ := json.Marshal(response)
	return http.StatusUnauthorized, headers, body
}

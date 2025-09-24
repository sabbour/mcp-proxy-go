package mcp

import (
	"encoding/json"
)

// Message represents a raw JSON-RPC message.
type Message struct {
	raw json.RawMessage
}

// NewMessage wraps the raw JSON bytes as a Message instance.
func NewMessage(raw []byte) Message {
	dup := make([]byte, len(raw))
	copy(dup, raw)
	return Message{raw: json.RawMessage(dup)}
}

// Bytes returns the raw JSON payload for the message.
func (m Message) Bytes() []byte {
	dup := make([]byte, len(m.raw))
	copy(dup, m.raw)
	return dup
}

// MarshalJSON implements json.Marshaler.
func (m Message) MarshalJSON() ([]byte, error) {
	return m.Bytes(), nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (m *Message) UnmarshalJSON(data []byte) error {
	dup := make([]byte, len(data))
	copy(dup, data)
	m.raw = json.RawMessage(dup)
	return nil
}

// Request is a minimal JSON-RPC request representation.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	ID      json.RawMessage `json:"id,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// ResponseError represents a JSON-RPC error object.
type ResponseError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Response is a minimal JSON-RPC response representation.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ResponseError  `json:"error,omitempty"`
}

// IsInitializeRequest returns true when the raw JSON message is an initialize request.
func IsInitializeRequest(raw []byte) bool {
	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		return false
	}

	return req.Method == "initialize" && req.JSONRPC == "2.0"
}

// IsNotification returns true when the message lacks an id field.
func IsNotification(raw []byte) bool {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return false
	}

	_, hasID := obj["id"]
	return !hasID
}

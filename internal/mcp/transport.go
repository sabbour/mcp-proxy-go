package mcp

import (
	"context"
)

// Transport defines the minimal interface shared by all MCP transports.
type Transport interface {
	Start(ctx context.Context) error
	Send(ctx context.Context, message Message) error
	Close() error
	OnMessage(func(Message))
	OnError(func(error))
	OnClose(func())
}

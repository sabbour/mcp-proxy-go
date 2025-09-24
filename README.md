# MCP Proxy Go

A Go port of the [Model Context Protocol (MCP) Proxy](https://github.com/punkpeye/mcp-proxy) that enables HTTP access to MCP servers running over stdio.

## ⚠️ Important Disclaimers

- **AI-Generated Code**: This Go implementation was generated with AI assistance and may contain bugs or incomplete features
- **Experimental Status**: This is an experimental port and should not be considered production-ready without thorough testing
- **Original Work**: This is a port of the original [mcp-proxy TypeScript implementation](https://github.com/punkpeye/mcp-proxy) by [punkpeye](https://github.com/punkpeye)
- **No Warranties**: Use at your own risk - see [LICENSE](LICENSE) for full disclaimer

## Attribution

This Go implementation is based on the original TypeScript [mcp-proxy](https://github.com/punkpeye/mcp-proxy) project by [punkpeye](https://github.com/punkpeye). The core architecture, API design, and protocol handling logic have been adapted from the original work.

## About MCP Proxy

MCP Proxy exposes an HTTP/SSE bridge that connects Model Context Protocol (MCP) clients to stdio-based MCP servers with optional resumability, authentication, and stateless operation.

## Features

- HTTP streamable and Server-Sent Events (SSE) transports adapted from the MCP SDK patterns
- Session tracking with event replay capabilities (experimental implementation)
- Optional stateless mode for simplified request handling
- API key authentication middleware adapted from the TypeScript reference
- Stdio transport for spawning MCP-compatible processes
- Basic test coverage and fixture servers for development
- Cross-platform binary builds via GitHub Actions

> **Note**: Feature parity with the original TypeScript implementation is not guaranteed. Some edge cases or advanced features may not be fully implemented.

## Getting Started

```bash
cd mcp-proxy-go
# Run the proxy against the sample fixture
GO_RUN_CMD="go run ./cmd/mcp-proxy --command go --args run,./fixtures/simple_stdio_server.go" 
eval "$GO_RUN_CMD"
```

Once running, issue MCP JSON-RPC requests against `http://localhost:3000/mcp`.
The `/sse` endpoint exposes the resumable event stream, and `/ping` offers a basic health check.

## Running Tests

All tests are centralized in the `tests/` folder:

```bash
cd mcp-proxy-go
go test ./tests     # Run all tests
go test ./...       # Run all tests (includes empty internal packages)
```

## CLI Flags

| Flag | Description | Default |
| --- | --- | --- |
| `--host` | Interface to bind | `::` |
| `--port` | Port for HTTP server | `3000` |
| `--api-key` | Optional API key to require on requests | `""` |
| `--command` | Command to launch the MCP server over stdio | _(required)_ |
| `--args` | Comma-separated command arguments | `""` |
| `--cwd` | Working directory for the subprocess | `""` |
| `--env` | Comma-separated `KEY=VALUE` env entries | `""` |
| `--stateless` | Enable stateless request handling | `false` |
| `--version` | Show version and build information | `false` |
| `--verbose` | Enable verbose debug logging | `false` |
| `--quiet` | Suppress all output except errors | `false` |

## Development Status

This is an **experimental port** with the following considerations:

- ✅ Basic MCP protocol implementation working
- ✅ HTTP/JSON-RPC endpoint functional  
- ✅ Session management implemented
- ✅ Authentication middleware working
- ⚠️  SSE transport needs more testing
- ⚠️  Error handling may be incomplete
- ⚠️  Edge cases not fully covered
- ❓ Performance characteristics unknown

## Testing

```bash
# Run tests
go test ./tests/...

# Test 
 ./mcp-proxy --host 127.0.0.1 --port 3001 -command "go run fixtures/time_server.go"

# Test with curl
curl -X POST http://localhost:3001/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}'
```

## Building for Multiple Platforms

The project includes automated cross-compilation:

```bash
# Local optimized build for all platforms
./build-optimized.sh

# GitHub Actions will build on tags
git tag v1.0.0
git push origin v1.0.0
```

## Project Layout

```
cmd/mcp-proxy       CLI entry point
fixtures/          Example stdio MCP server used in tests
tests/             Centralized test files for all internal packages
internal/auth      API key middleware
internal/eventstore In-memory event store backing resumability
internal/httpserver HTTP and SSE server implementation
internal/jsonfilter Filter for process stdout to drop non-JSON lines
internal/mcp       Minimal MCP transport abstractions
internal/proxy     Transport bridge (currently unused by CLI)
internal/stdio     Stdio client transport
```

## Parity Notes

- The Go implementation mirrors the HTTP contract, authentication, and resumability
  options from `mcp-proxy-ts`.
- The sample fixture reproduces the same resources exposed by the TypeScript tests.
- Additional transports (e.g., tap transport) can be implemented on top of the `mcp.Transport`
  interface introduced here.

## Docker Usage

You can build and run the MCP proxy in a minimal container using Docker:

```bash
# Build the image
docker build -t mcp-proxy-go .

# Run the proxy (example: with a fixture server)
docker run -p 3000:3000 mcp-proxy-go --command "go run fixtures/simple_stdio_server.go"
```

The default Dockerfile uses a multi-stage build and a `scratch` final image for maximum security and minimal size. Only the compiled binary, README, and LICENSE are included in the final image.

> **Note:** If you want to use a different fixture or server, mount it or build a custom image.

## Using MCP Proxy Go as a Base Image for Custom MCP Servers

You can use the `mcp-proxy-go` Docker image as a base for your own Dockerfiles to run custom MCP servers (e.g., built with Node.js/NPM).

### Example: Multi-stage Dockerfile with Node.js MCP Server

```dockerfile
# Stage 1: Build your Node.js MCP server
FROM node:20-alpine AS builder
WORKDIR /app
COPY . .
RUN npm install && npm run build

# Stage 2: Use mcp-proxy-go as the base
FROM mcp-proxy-go:latest
WORKDIR /app
COPY --from=builder /app/dist /app/server
COPY --from=builder /app/package.json /app/server/
COPY --from=builder /app/node_modules /app/server/node_modules

# Optionally override entrypoint to run both processes
ENTRYPOINT ["/app/mcp-proxy", "--command", "node", "--args", "server/index.js"]
```

This lets you combine the Go proxy with your own MCP server built using NPM. You can also mount your MCP server code at runtime:

```sh
docker run -p 3000:3000 -v $(pwd)/my-server:/app/server mcp-proxy-go:latest --command "node" --args "server/index.js"
```
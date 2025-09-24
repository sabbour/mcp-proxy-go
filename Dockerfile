# Build stage
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -ldflags="-s -w" -o mcp-proxy ./cmd/mcp-proxy

# Final image (scratch)
FROM scratch
WORKDIR /app
COPY --from=builder /app/mcp-proxy ./mcp-proxy
COPY README.md ./README.md
COPY LICENSE ./LICENSE
EXPOSE 3000
ENTRYPOINT ["/app/mcp-proxy"]

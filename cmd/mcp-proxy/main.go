// MCP Proxy Go - HTTP proxy for Model Context Protocol servers
//
// This is a Go port of the original TypeScript mcp-proxy implementation:
// https://github.com/punkpeye/mcp-proxy
//
// Original work Copyright (c) 2024 punkpeye
// Go port implementation generated with AI assistance
// 
// This implementation adapts the core architecture and API design from the
// original TypeScript project to provide HTTP/SSE access to stdio-based MCP servers.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/sabbour/mcp-proxy-go/internal/eventstore"
	"github.com/sabbour/mcp-proxy-go/internal/httpserver"
	"github.com/sabbour/mcp-proxy-go/internal/mcp"
	"github.com/sabbour/mcp-proxy-go/internal/stdio"
)

// Build-time variables (set by ldflags)
var (
	Version   = "dev"
	BuildTime = "unknown"
	CommitSHA = "unknown"
)

func main() {
	var (
		host      = flag.String("host", "0.0.0.0", "Host interface to bind the HTTP server")
		port      = flag.Int("port", 3000, "Port for the HTTP server")
		apiKey    = flag.String("api-key", "", "Optional API key required for incoming requests")
		command   = flag.String("command", "", "Command to launch the MCP server over stdio")
		argsList  = flag.String("args", "", "Comma-separated list of arguments for the command")
		cwd       = flag.String("cwd", "", "Working directory for the launched command")
		envList   = flag.String("env", "", "Comma-separated list of KEY=VALUE pairs to add to the environment")
		stateless = flag.Bool("stateless", false, "Enable stateless mode (no session reuse)")
		verbose   = flag.Bool("verbose", false, "Enable verbose debug logging")
		quiet     = flag.Bool("quiet", false, "Suppress all debug output except errors")
		version   = flag.Bool("version", false, "Show version information")
	)

	flag.Parse()

	// Handle version flag
	if *version {
		fmt.Printf("MCP Proxy Go\n")
		fmt.Printf("Version: %s\n", Version)
		fmt.Printf("Build Time: %s\n", BuildTime)
		fmt.Printf("Commit: %s\n", CommitSHA)
		fmt.Printf("Go Version: %s\n", runtime.Version())
		fmt.Printf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		fmt.Printf("\nBased on the original TypeScript implementation:\n")
		fmt.Printf("https://github.com/punkpeye/mcp-proxy\n")
		fmt.Printf("\nGenerated with AI assistance - use at your own risk\n")
		return
	}

	// Set up logging based on verbosity flags
	if *quiet {
		// Only show errors
		log.SetOutput(io.Discard)
	}

	logDebug := func(format string, args ...interface{}) {
		if !*quiet {
			log.Printf("[mcp-proxy] DEBUG: "+format, args...)
		}
	}

	logInfo := func(format string, args ...interface{}) {
		if !*quiet {
			log.Printf("[mcp-proxy] INFO: "+format, args...)
		}
	}

	logError := func(format string, args ...interface{}) {
		log.Printf("[mcp-proxy] ERROR: "+format, args...)
	}

	if *verbose {
		logDebug("Verbose logging enabled")
	} else if *quiet {
		// Re-enable log output just for errors
		log.SetOutput(os.Stderr)
	}

	logDebug("Starting with command=%s, args=%s, port=%d, host=%s", *command, *argsList, *port, *host)

	if *command == "" {
		logError("--command is required")
		fmt.Fprintln(os.Stderr, "--command is required")
		os.Exit(2)
	}

	args := splitCommaList(*argsList)
	env := splitCommaList(*envList)

	// Parse the command to separate the executable from its arguments
	cmdParts := strings.Fields(*command)
	if len(cmdParts) == 0 {
		log.Println("[mcp-proxy] ERROR: --command is empty")
		fmt.Fprintln(os.Stderr, "--command is empty")
		os.Exit(2)
	}
	
	actualCommand := cmdParts[0]
	cmdArgs := cmdParts[1:]
	
	// If args were provided via -args flag, append them to the command args
	if len(args) > 0 {
		cmdArgs = append(cmdArgs, args...)
	}

	if *verbose {
		logDebug("Parsed command: %s", actualCommand)
		logDebug("Parsed command args: %v", cmdArgs)
		logDebug("Parsed env: %v", env)
	}

	server, err := httpserver.Start(httpserver.Options{
		Host:   *host,
		Port:   *port,
		APIKey: *apiKey,
		CreateTransport: func(ctx context.Context, req *http.Request) (mcp.Transport, error) {
			if *verbose {
				logDebug("Creating transport for request from %s to %s", req.RemoteAddr, req.URL.Path)
			}
			params := stdio.Params{
				Command: actualCommand,
				Args:    cmdArgs,
				Dir:     *cwd,
				Env:     env,
			}
			if *verbose {
				logDebug("Creating stdio client with params: %+v", params)
			}
			transport := stdio.NewClient(params)
			if *verbose {
				logDebug("Successfully created stdio client")
			}
			return transport, nil
		},
		EventStoreFactory: func() *eventstore.Memory {
			return eventstore.NewMemory()
		},
		Stateless: *stateless,
		OnConnect: func(sessionID string) {
			if *verbose {
				logDebug("session %s connected", sessionID)
			}
		},
		OnClose: func(sessionID string) {
			if *verbose {
				logDebug("session %s closed", sessionID)
			}
		},
		OnUnhandled: func(w http.ResponseWriter, r *http.Request) {
			if *verbose {
				logDebug("Unhandled request: %s %s", r.Method, r.URL.Path)
				logDebug("Request headers: %+v", r.Header)
				
				// Log request body for POST requests
				if r.Method == "POST" {
					if body, err := io.ReadAll(r.Body); err == nil {
						logDebug("Request body: %s", string(body))
						// Reset body for further processing
						r.Body = io.NopCloser(bytes.NewReader(body))
					}
				}
			}
			
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(fmt.Sprintf("Endpoint not found: %s %s. Try /mcp", r.Method, r.URL.Path)))
		},
	})
	if err != nil {
		logError("failed to start http server: %v", err)
		log.Fatalf("[mcp-proxy] ERROR: failed to start http server: %v", err)
	}

	logInfo("listening on %s:%d", *host, *port)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logInfo("shutting down")
	if err := server.Close(context.Background()); err != nil {
		logError("shutdown error: %v", err)
	}
}

func splitCommaList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	var result []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

package stdio

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"

	"github.com/sabbour/mcp-proxy-go/internal/jsonfilter"
	"github.com/sabbour/mcp-proxy-go/internal/mcp"
)

// Params configures the stdio client transport.
type Params struct {
	Command string
	Args    []string
	Dir     string
	Env     []string
}

type Client struct {
	params     Params
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	stderr     io.ReadCloser
	mu         sync.Mutex
	onMessage  func(mcp.Message)
	onError    func(error)
	onClose    func()
	closedOnce sync.Once
}

// NewClient creates a new stdio client transport.
func NewClient(params Params) *Client {
	return &Client{params: params}
}

// OnMessage registers a callback for inbound messages.
func (c *Client) OnMessage(fn func(mcp.Message)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onMessage = fn
}

// OnError registers a callback for transport errors.
func (c *Client) OnError(fn func(error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onError = fn
}

// OnClose registers a callback invoked when the process exits.
func (c *Client) OnClose(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onClose = fn
}

// Start launches the underlying process and begins reading stdout.
func (c *Client) Start(ctx context.Context) error {
	log.Printf("[mcp-proxy] DEBUG: Starting stdio client with command: %s %v", c.params.Command, c.params.Args)
	
	c.mu.Lock()
	if c.cmd != nil {
		c.mu.Unlock()
		return errors.New("already started")
	}

	cmd := exec.CommandContext(ctx, c.params.Command, c.params.Args...)
	if c.params.Dir != "" {
		cmd.Dir = c.params.Dir
		log.Printf("[mcp-proxy] DEBUG: Set working directory: %s", c.params.Dir)
	}
	if len(c.params.Env) > 0 {
		cmd.Env = append(os.Environ(), c.params.Env...)
		log.Printf("[mcp-proxy] DEBUG: Added environment variables: %v", c.params.Env)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Printf("[mcp-proxy] DEBUG: Error creating stdin pipe: %v", err)
		c.mu.Unlock()
		return err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("[mcp-proxy] DEBUG: Error creating stdout pipe: %v", err)
		c.mu.Unlock()
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("[mcp-proxy] DEBUG: Error creating stderr pipe: %v", err)
		c.mu.Unlock()
		return err
	}

	c.cmd = cmd
	c.stdin = stdin
	c.stdout = stdout
	c.stderr = stderr
	c.mu.Unlock()

	if err := cmd.Start(); err != nil {
		log.Printf("[mcp-proxy] DEBUG: Error starting command: %v", err)
		return err
	}

	log.Printf("[mcp-proxy] DEBUG: Command started successfully with PID: %d", cmd.Process.Pid)

	go c.readStdout()
	go c.readStderr()
	go func() {
		err := cmd.Wait()
		log.Printf("[mcp-proxy] DEBUG: Command finished with error: %v", err)
		c.close()
	}()

	return nil
}

func (c *Client) readStdout() {
	log.Printf("[mcp-proxy] DEBUG: Starting to read stdout")
	reader := bufio.NewReader(jsonfilter.NewReader(c.stdout))
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			log.Printf("[mcp-proxy] DEBUG: Read stdout line: %s", string(line))
			trimmed := bytesTrim(line)
			if len(trimmed) > 0 {
				log.Printf("[mcp-proxy] DEBUG: Processing message: %s", string(trimmed))
				msg := mcp.NewMessage(trimmed)
				c.mu.Lock()
				onMessage := c.onMessage
				c.mu.Unlock()
				if onMessage != nil {
					log.Printf("[mcp-proxy] DEBUG: Calling onMessage handler")
					onMessage(msg)
				} else {
					log.Printf("[mcp-proxy] DEBUG: No onMessage handler set")
				}
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Printf("[mcp-proxy] DEBUG: Error reading stdout: %v", err)
				c.reportError(err)
			} else {
				log.Printf("[mcp-proxy] DEBUG: Reached EOF on stdout")
			}
			return
		}
	}
}

func (c *Client) readStderr() {
	scanner := bufio.NewScanner(c.stderr)
	for scanner.Scan() {
		// The JSON filter already logs non-JSON output, but stderr is forwarded as errors.
		c.reportError(errors.New(scanner.Text()))
	}
}

func (c *Client) reportError(err error) {
	c.mu.Lock()
	onError := c.onError
	c.mu.Unlock()
	if onError != nil {
		onError(err)
	}
}

// Send writes the JSON message to stdin.
func (c *Client) Send(ctx context.Context, msg mcp.Message) error {
	log.Printf("[mcp-proxy] DEBUG: Sending message: %s", string(msg.Bytes()))
	
	c.mu.Lock()
	stdin := c.stdin
	c.mu.Unlock()

	if stdin == nil {
		log.Printf("[mcp-proxy] DEBUG: No stdin available for sending")
		return errors.New("stdin not initialized")
	}

	data := msg.Bytes()
	data = append(data, '\n')

	_, err := stdin.Write(data)
	if err != nil {
		log.Printf("[mcp-proxy] DEBUG: Error writing to stdin: %v", err)
	} else {
		log.Printf("[mcp-proxy] DEBUG: Successfully wrote message to stdin")
	}
	return err
}

// Close terminates the process.
func (c *Client) Close() error {
	c.close()
	return nil
}

func (c *Client) close() {
	c.closedOnce.Do(func() {
		c.mu.Lock()
		onClose := c.onClose
		stdin := c.stdin
		cmd := c.cmd
		c.stdin = nil
		c.cmd = nil
		c.mu.Unlock()

		if stdin != nil {
			_ = stdin.Close()
		}
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}

		if onClose != nil {
			onClose()
		}
	})
}

// Helper to trim trailing whitespace while keeping JSON intact.
func bytesTrim(b []byte) []byte {
	return bytes.TrimSpace(b)
}

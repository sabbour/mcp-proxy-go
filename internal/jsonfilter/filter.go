package jsonfilter

import (
	"bytes"
	"io"
	"log"
)

// Reader filters non-JSON lines from an underlying stream.
type Reader struct {
	source  io.Reader
	buffer  bytes.Buffer
	pending bytes.Buffer
}

// NewReader wraps r with filtering behavior.
func NewReader(r io.Reader) *Reader {
	return &Reader{source: r}
}

// Read implements io.Reader by returning only newline-delimited JSON lines.
func (r *Reader) Read(p []byte) (int, error) {
	if r.pending.Len() > 0 {
		return r.pending.Read(p)
	}

	tmp := make([]byte, len(p))
	n, err := r.source.Read(tmp)
	if n > 0 {
		r.buffer.Write(tmp[:n])
		r.flushBufferedLines()
	}

	if r.pending.Len() > 0 {
		return r.pending.Read(p)
	}

	if err == io.EOF {
		// Process any remaining buffer content on EOF
		if r.buffer.Len() > 0 {
			remaining := r.buffer.String()
			r.buffer.Reset()
			trimmed := bytes.TrimSpace([]byte(remaining))
			if len(trimmed) > 0 && trimmed[0] == '{' {
				r.pending.Write(trimmed)
				r.pending.WriteByte('\n')
			} else if len(trimmed) > 0 {
				log.Printf("[mcp-proxy] ignoring non-JSON output: %s", remaining)
			}
		}
		if r.pending.Len() > 0 {
			return r.pending.Read(p)
		}
		return 0, io.EOF
	}

	if err != nil {
		return 0, err
	}

	return 0, nil
}

func (r *Reader) flushBufferedLines() {
	for {
		line, err := r.buffer.ReadString('\n')
		if err == io.EOF {
			r.buffer.WriteString(line)
			return
		}

		trimmed := bytes.TrimSpace([]byte(line))
		if len(trimmed) == 0 {
			continue
		}

		if trimmed[0] == '{' {
			r.pending.Write(trimmed)
			r.pending.WriteByte('\n')
			continue
		}

		log.Printf("[mcp-proxy] ignoring non-JSON output: %s", line)
	}
}

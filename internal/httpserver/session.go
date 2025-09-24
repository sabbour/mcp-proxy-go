package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/sabbour/mcp-proxy-go/internal/eventstore"
	"github.com/sabbour/mcp-proxy-go/internal/mcp"
)

type session struct {
	id        string
	transport mcp.Transport
	pending   sync.Map // id(string) -> chan mcp.Message
	events    chan eventstore.Event
	subsMu    sync.Mutex
	subs      map[chan eventstore.Event]struct{}
	store     *eventstore.Memory
	ctx       context.Context
	cancel    context.CancelFunc
	onClose   func()
	closeOnce sync.Once
}

func newSession(id string, transport mcp.Transport, store *eventstore.Memory, onClose func()) *session {
	ctx, cancel := context.WithCancel(context.Background())
	s := &session{
		id:        id,
		transport: transport,
		events:    make(chan eventstore.Event, 128),
		subs:      map[chan eventstore.Event]struct{}{},
		store:     store,
		ctx:       ctx,
		cancel:    cancel,
		onClose:   onClose,
	}

	transport.OnMessage(s.handleMessage)
	transport.OnError(func(err error) {
		s.broadcast(eventstore.Event{
			ID:       "",
			StreamID: id,
			Payload:  buildErrorMessage(err),
		})
	})
	transport.OnClose(func() {
		s.cancel()
		s.closeOnce.Do(func() {
			if s.onClose != nil {
				s.onClose()
			}
		})
	})

	go s.run()

	return s
}

func (s *session) start(ctx context.Context) error {
	return s.transport.Start(ctx)
}

func (s *session) close() error {
	s.cancel()
	s.closeOnce.Do(func() {
		if s.onClose != nil {
			s.onClose()
		}
	})
	return s.transport.Close()
}

func (s *session) handleMessage(msg mcp.Message) {
	raw := msg.Bytes()

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		s.storeAndBroadcast(raw)
		return
	}

	idRaw, hasID := envelope["id"]

	if hasID {
		idKey := string(idRaw)
		if ch, ok := s.pending.Load(idKey); ok {
			ch.(chan mcp.Message) <- msg
			s.storeAndBroadcast(raw)
			return
		}
	}

	s.storeAndBroadcast(raw)
}

func (s *session) storeAndBroadcast(payload []byte) {
	event := eventstore.Event{StreamID: s.id, Payload: payload}
	if s.store != nil {
		event.ID = s.store.Store(s.id, payload)
	}
	s.broadcast(event)
}

func (s *session) broadcast(event eventstore.Event) {
	s.subsMu.Lock()
	for ch := range s.subs {
		select {
		case ch <- event:
		default:
		}
	}
	s.subsMu.Unlock()
}

func (s *session) subscribe(ch chan eventstore.Event) func() {
	s.subsMu.Lock()
	s.subs[ch] = struct{}{}
	s.subsMu.Unlock()

	return func() {
		s.subsMu.Lock()
		delete(s.subs, ch)
		s.subsMu.Unlock()
	}
}

func (s *session) request(ctx context.Context, payload []byte) ([]byte, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	idRaw, hasID := envelope["id"]
	var ch chan mcp.Message
	if hasID {
		ch = make(chan mcp.Message, 1)
		s.pending.Store(string(idRaw), ch)
	}

	err := s.transport.Send(ctx, mcp.NewMessage(payload))
	if err != nil {
		if hasID {
			s.pending.Delete(string(idRaw))
		}
		return nil, err
	}

	if !hasID {
		return nil, nil
	}

	select {
	case <-ctx.Done():
		s.pending.Delete(string(idRaw))
		return nil, ctx.Err()
	case msg := <-ch:
		s.pending.Delete(string(idRaw))
		return msg.Bytes(), nil
	}
}

func (s *session) replayAfter(lastID string, fn func(eventstore.Event)) {
	if s.store == nil {
		return
	}
	s.store.ReplayAfter(lastID, fn)
}

func (s *session) run() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if s.store != nil {
				s.storeAndBroadcast(buildHeartbeat())
			}
		}
	}
}

func buildErrorMessage(err error) []byte {
	payload := map[string]any{
		"jsonrpc": "2.0",
		"method":  "mcp-proxy/error",
		"params": map[string]any{
			"message": err.Error(),
		},
	}

	raw, _ := json.Marshal(payload)
	return raw
}

func buildHeartbeat() []byte {
	payload := map[string]any{
		"jsonrpc": "2.0",
		"method":  "mcp-proxy/heartbeat",
		"params": map[string]any{
			"at": time.Now().UTC().Format(time.RFC3339Nano),
		},
	}

	raw, _ := json.Marshal(payload)
	return raw
}

func readRequestBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		return nil, err
	}

	if len(body) == 0 {
		return nil, errors.New("empty body")
	}

	return body, nil
}

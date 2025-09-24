package eventstore

import (
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Event holds the payload for a stored message.
type Event struct {
	ID        string
	StreamID  string
	Payload   []byte
	Timestamp time.Time
}

// Memory implements an in-memory event store with resumability support.
type Memory struct {
	mu     sync.RWMutex
	events map[string]Event
}

// NewMemory creates a new in-memory event store.
func NewMemory() *Memory {
	return &Memory{events: map[string]Event{}}
}

// Store adds a new event to the store and returns the generated event ID.
func (m *Memory) Store(streamID string, payload []byte) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	eventID := m.generateID(streamID)
	dup := make([]byte, len(payload))
	copy(dup, payload)
	m.events[eventID] = Event{ID: eventID, StreamID: streamID, Payload: dup, Timestamp: time.Now()}
	return eventID
}

// ReplayAfter replays events for the same stream after the provided event ID.
func (m *Memory) ReplayAfter(lastEventID string, fn func(Event)) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	event, ok := m.events[lastEventID]
	if !ok {
		return ""
	}

	streamID := event.StreamID

	var streamEvents []Event
	for _, e := range m.events {
		if e.StreamID == streamID {
			streamEvents = append(streamEvents, e)
		}
	}

	sort.Slice(streamEvents, func(i, j int) bool {
		return streamEvents[i].Timestamp.Before(streamEvents[j].Timestamp)
	})

	found := false
	for _, e := range streamEvents {
		if e.ID == lastEventID {
			found = true
			continue
		}

		if found {
			fn(e)
		}
	}

	return streamID
}

func (m *Memory) generateID(streamID string) string {
	return streamID + "_" + uuid.NewString()
}

package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sabbour/mcp-proxy-go/internal/eventstore"
)

func TestMemoryEventStore(t *testing.T) {
	t.Run("store and replay events", func(t *testing.T) {
		store := eventstore.NewMemory()
		streamID := "test-stream"
		
		// Store some events
		payload1 := []byte(`{"id": 1, "data": "first"}`)
		payload2 := []byte(`{"id": 2, "data": "second"}`)
		payload3 := []byte(`{"id": 3, "data": "third"}`)
		
		eventID1 := store.Store(streamID, payload1)
		time.Sleep(time.Millisecond) // Ensure timestamp ordering
		eventID2 := store.Store(streamID, payload2)
		time.Sleep(time.Millisecond) // Ensure timestamp ordering
		eventID3 := store.Store(streamID, payload3)
		
		require.NotEmpty(t, eventID1)
		require.NotEmpty(t, eventID2)
		require.NotEmpty(t, eventID3)
		require.Contains(t, eventID1, streamID)
		require.Contains(t, eventID2, streamID)
		require.Contains(t, eventID3, streamID)
		
		// Replay events after the first one
		var replayed []eventstore.Event
		resultStreamID := store.ReplayAfter(eventID1, func(e eventstore.Event) {
			replayed = append(replayed, e)
		})
		
		require.Equal(t, streamID, resultStreamID)
		require.Len(t, replayed, 2)
		require.Equal(t, eventID2, replayed[0].ID)
		require.Equal(t, payload2, replayed[0].Payload)
		require.Equal(t, eventID3, replayed[1].ID)
		require.Equal(t, payload3, replayed[1].Payload)
	})

	t.Run("replay with non-existent event returns empty", func(t *testing.T) {
		store := eventstore.NewMemory()
		
		resultStreamID := store.ReplayAfter("non-existent", func(e eventstore.Event) {
			t.Fatal("should not call replay function")
		})
		
		require.Empty(t, resultStreamID)
	})

	t.Run("isolates streams", func(t *testing.T) {
		store := eventstore.NewMemory()
		
		// Store events in different streams
		eventA1 := store.Store("stream-a", []byte(`{"stream": "a", "seq": 1}`))
		_ = store.Store("stream-b", []byte(`{"stream": "b", "seq": 1}`))
		_ = store.Store("stream-a", []byte(`{"stream": "a", "seq": 2}`))
		
		// Replay stream A events
		var replayedA []eventstore.Event
		store.ReplayAfter(eventA1, func(e eventstore.Event) {
			replayedA = append(replayedA, e)
		})
		
		// Should only get stream A events, not stream B
		require.Len(t, replayedA, 1)
		require.Equal(t, "stream-a", replayedA[0].StreamID)
		require.Contains(t, string(replayedA[0].Payload), `"stream": "a"`)
		require.Contains(t, string(replayedA[0].Payload), `"seq": 2`)
	})
}
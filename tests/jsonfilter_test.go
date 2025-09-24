package tests

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sabbour/mcp-proxy-go/internal/jsonfilter"
)

func TestJSONFilterReader(t *testing.T) {
	t.Run("filters out non-JSON lines", func(t *testing.T) {
		input := `{"valid": "json"}
This is not JSON
{"another": "valid"}
Error: something went wrong
{"third": "json"}`

		reader := jsonfilter.NewReader(strings.NewReader(input))
		
		var buf bytes.Buffer
		_, err := buf.ReadFrom(reader)
		require.NoError(t, err)
		
		result := buf.String()
		lines := strings.Split(strings.TrimSpace(result), "\n")
		
		// Should only contain JSON lines
		require.Contains(t, result, `{"valid": "json"}`)
		require.Contains(t, result, `{"another": "valid"}`)
		require.Contains(t, result, `{"third": "json"}`)
		require.NotContains(t, result, "This is not JSON")
		require.NotContains(t, result, "Error: something went wrong")
		
		// Verify we have exactly 3 lines of output
		nonEmptyLines := 0
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				nonEmptyLines++
			}
		}
		require.Equal(t, 3, nonEmptyLines)
	})

	t.Run("handles empty input", func(t *testing.T) {
		reader := jsonfilter.NewReader(strings.NewReader(""))
		
		var buf bytes.Buffer
		_, err := buf.ReadFrom(reader)
		require.NoError(t, err)
		
		require.Empty(t, buf.String())
	})

	t.Run("handles only non-JSON input", func(t *testing.T) {
		input := `This is not JSON
Another line
Error message`

		reader := jsonfilter.NewReader(strings.NewReader(input))
		
		var buf bytes.Buffer
		_, err := buf.ReadFrom(reader)
		require.NoError(t, err)
		
		require.Empty(t, strings.TrimSpace(buf.String()))
	})
}
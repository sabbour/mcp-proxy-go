package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type resourceReadParams struct {
	URI string `json:"uri"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()

		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		if len(req.ID) == 0 {
			continue
		}

		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      json.RawMessage(req.ID),
		}

		switch req.Method {
		case "initialize":
			resp["result"] = map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]any{
					"resources": map[string]any{
						"subscribe": false,
					},
				},
				"serverInfo": map[string]any{
					"name":    "time-date-server",
					"version": "1.0.0",
				},
			}

		case "resources/list":
			resp["result"] = map[string]any{
				"resources": []any{
					map[string]any{
						"name":        "Current Date and Time",
						"uri":         "time://current",
						"description": "Current date and time in local timezone",
						"mimeType":    "text/plain",
					},
					map[string]any{
						"name":        "Current UTC Time",
						"uri":         "time://utc",
						"description": "Current date and time in UTC",
						"mimeType":    "text/plain",
					},
					map[string]any{
						"name":        "Unix Timestamp",
						"uri":         "time://unix",
						"description": "Current Unix timestamp",
						"mimeType":    "text/plain",
					},
					map[string]any{
						"name":        "ISO 8601 Format",
						"uri":         "time://iso8601",
						"description": "Current time in ISO 8601 format",
						"mimeType":    "text/plain",
					},
					map[string]any{
						"name":        "Human Readable",
						"uri":         "time://human",
						"description": "Human-friendly date and time",
						"mimeType":    "text/plain",
					},
				},
			}

		case "resources/read":
			var params resourceReadParams
			if err := json.Unmarshal(req.Params, &params); err != nil {
				resp["error"] = map[string]any{
					"code":    -32602,
					"message": "Invalid params: " + err.Error(),
				}
				break
			}

			now := time.Now()
			var content string

			switch params.URI {
			case "time://current":
				content = fmt.Sprintf("Current Local Time: %s", now.Format("Monday, January 2, 2006 at 3:04:05 PM MST"))

			case "time://utc":
				content = fmt.Sprintf("Current UTC Time: %s", now.UTC().Format("Monday, January 2, 2006 at 15:04:05 UTC"))

			case "time://unix":
				content = fmt.Sprintf("Unix Timestamp: %d\nSeconds since January 1, 1970 UTC", now.Unix())

			case "time://iso8601":
				content = fmt.Sprintf("ISO 8601 Format: %s", now.Format(time.RFC3339))

			case "time://human":
				weekday := now.Format("Monday")
				month := now.Format("January")
				day := now.Format("2")
				year := now.Format("2006")
				timeStr := now.Format("3:04:05 PM")
				timezone := now.Format("MST")
				
				content = fmt.Sprintf(`Today is %s, %s %s, %s

The current time is %s %s

Some additional time information:
- Day of year: %d
- Week of year: %d
- Time zone offset: %s
- Daylight saving: %t`,
					weekday, month, day, year,
					timeStr, timezone,
					now.YearDay(),
					getWeekOfYear(now),
					now.Format("-07:00"),
					isDST(now))

			default:
				resp["error"] = map[string]any{
					"code":    -32601,
					"message": fmt.Sprintf("Unknown resource URI: %s", params.URI),
				}
				break
			}

			if content != "" {
				resp["result"] = map[string]any{
					"contents": []any{
						map[string]any{
							"uri":      params.URI,
							"mimeType": "text/plain",
							"text":     content,
						},
					},
				}
			}

		case "resources/templates/list":
			resp["result"] = map[string]any{
				"resourceTemplates": []any{
					map[string]any{
						"name":        "Time in specific timezone",
						"description": "Get current time in a specific timezone (e.g., America/New_York, Europe/London)",
						"uriTemplate": "time://timezone/{timezone}",
					},
					map[string]any{
						"name":        "Custom time format",
						"description": "Get current time with custom Go time format",
						"uriTemplate": "time://format/{format}",
					},
				},
			}

		case "resources/subscribe", "resources/unsubscribe":
			resp["result"] = map[string]any{}

		default:
			resp["error"] = map[string]any{
				"code":    -32601,
				"message": fmt.Sprintf("method %s not found", req.Method),
			}
		}

		if err := json.NewEncoder(os.Stdout).Encode(resp); err != nil {
			return
		}
	}
}

// Helper function to get week of year
func getWeekOfYear(t time.Time) int {
	_, week := t.ISOWeek()
	return week
}

// Helper function to detect daylight saving time
func isDST(t time.Time) bool {
	// Get timezone info
	zone, offset := t.Zone()
	
	// Check if we're in a timezone that uses DST by comparing with standard time
	january := time.Date(t.Year(), 1, 1, 12, 0, 0, 0, t.Location())
	_, janOffset := january.Zone()
	
	july := time.Date(t.Year(), 7, 1, 12, 0, 0, 0, t.Location())
	_, julyOffset := july.Zone()
	
	// If the offsets are different, DST is used in this timezone
	if janOffset == julyOffset {
		return false // No DST in this timezone
	}
	
	// DST is active if current offset differs from standard time
	stdOffset := janOffset
	if julyOffset > janOffset {
		stdOffset = julyOffset
	}
	
	_ = zone // Use zone to avoid unused variable
	return offset != stdOffset
}
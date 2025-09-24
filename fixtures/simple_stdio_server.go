package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
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
						"subscribe": true,
					},
				},
				"serverInfo": map[string]any{
					"name":    "example-server",
					"version": "1.0.0",
				},
			}
		case "resources/list":
			resp["result"] = map[string]any{
				"resources": []any{
					map[string]any{
						"name": "Example Resource",
						"uri":  "file:///example.txt",
					},
				},
			}
		case "resources/read":
			resp["result"] = map[string]any{
				"contents": []any{
					map[string]any{
						"uri":      "file:///example.txt",
						"mimeType": "text/plain",
						"text":     "This is the content of the example resource.",
					},
				},
			}
		case "resources/templates/list":
			resp["result"] = map[string]any{
				"resourceTemplates": []any{
					map[string]any{
						"name":        "Example resource template",
						"description": "Specify the filename to retrieve",
						"uriTemplate": "file://{filename}",
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

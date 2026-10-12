package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":[{"id":"claude-native-fixture","type":"model","display_name":"Native fixture","created_at":"2026-01-01T00:00:00Z"}],"has_more":false,"first_id":"claude-native-fixture","last_id":"claude-native-fixture"}`)
	})
	mux.HandleFunc("POST /v1/messages/count_tokens", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "invalid count request", http.StatusBadRequest)
			return
		}
		_, _ = fmt.Fprintf(w, `{"input_tokens":%d}`, max(1, len(body)/4))
	})
	mux.HandleFunc("POST /v1/messages", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model    string          `json:"model"`
			Messages json.RawMessage `json:"messages"`
			System   json.RawMessage `json:"system"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(&request); err != nil {
			http.Error(w, "invalid fixture request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		events := []string{
			fmt.Sprintf(`{"type":"message_start","message":{"id":"msg_fixture","type":"message","role":"assistant","model":%q,"content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":0,"output_tokens":0}}}`, request.Model),
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Native fixture observation"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":0}}`,
			`{"type":"message_stop"}`,
		}

		capacityKind := ""
		switch {
		case strings.Contains(string(request.System), "You are compacting the earlier portion"):
		case strings.Contains(string(request.Messages), "Native single-item capacity."):
			capacityKind = "single-item"
		case strings.Contains(string(request.Messages), "Native multi-record capacity."):
			capacityKind = "multi-record"
		}
		size := map[string]int{"multi-record": 512 << 10, "single-item": 5 << 20}[capacityKind]
		if size > 0 {
			text := "capacity-begin:" + strings.Repeat("capacity data ", size/14) + ":capacity-end"
			completed := append([]string(nil), events[3:]...)
			deltaType, field := "text_delta", "text"
			if capacityKind == "single-item" {
				deltaType, field = "thinking_delta", "thinking"
				events[1] = `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`
				completed = append([]string{
					`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"native-capacity-signature"}}`,
					`{"type":"content_block_stop","index":0}`,
					`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
					`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Native fixture observation"}}`,
					`{"type":"content_block_stop","index":1}`,
				}, events[4:]...)
			}
			events = events[:2]
			for len(text) > 0 {
				count := min(len(text), 16<<10)
				events = append(events, fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":%q,%q:%q}}`, deltaType, field, text[:count]))
				text = text[count:]
			}
			events = append(events, completed...)
		}
		if strings.Contains(string(request.Messages), "Save a native reusable Skill.") && !strings.Contains(string(request.Messages), `"id":"native_skill_proposal"`) {
			name, id, arguments := "search_tools", "native_skill_discovery", `{"query":"select:propose_skill"}`
			if strings.Contains(string(request.Messages), `"id":"native_skill_discovery"`) {
				name, id, arguments = "propose_skill", "native_skill_proposal", `{"name":"native-proposal","description":"Review native plugin changes","instructions":"Inspect the canonical plugin boundary before publishing.","scope":"user"}`
			}
			events = []string{
				events[0],
				fmt.Sprintf(`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":%q,"name":%q,"input":{}}}`, id, name),
				fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":%q}}`, arguments),
				`{"type":"content_block_stop","index":0}`,
				`{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":0}}`,
				`{"type":"message_stop"}`,
			}
		}
		for _, event := range events {
			var value struct{ Type string }
			if err := json.Unmarshal([]byte(event), &value); err != nil {
				log.Fatal(err)
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", value.Type, event); err != nil {
				return
			}
		}
	})
	log.Fatal(http.ListenAndServe("127.0.0.1:18080", mux))
}

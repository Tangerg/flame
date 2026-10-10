package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":[{"id":"claude-native-fixture","type":"model","display_name":"Native fixture","created_at":"2026-01-01T00:00:00Z"}],"has_more":false,"first_id":"claude-native-fixture","last_id":"claude-native-fixture"}`)
	})
	mux.HandleFunc("POST /v1/messages", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&request); err != nil {
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

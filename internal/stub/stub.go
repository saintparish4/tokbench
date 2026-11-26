// Package stub is a fake OpenAI-compatible chat-completions server.
//
// It exists so the client, the runner and the CLI can be tested, and the
// walkthroughs in the guides can run, without a GPU or a real model. It is
// not a model of any provider's performance: the delays are whatever Options
// says, so no number measured against it means anything about a real system.
//
// The stream has the shape real servers produce: a role-only chunk first,
// then one text chunk per token up to the request's max_tokens, then a usage
// chunk with empty choices, then [DONE]. OmitUsage drops the usage chunk to
// imitate a provider that ignores stream_options.
package stub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Options shape the fake stream
type Options struct {
	FirstDelay time.Duration // wait before the first text chunk
	Gap        time.Duration // wait before each later text chunk
	OmitUsage  bool          // leave out the usage chunk
}

// Handler servers POST /v1/chat/completions as a server-sent-event stream
func Handler(o Options) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			MaxTokens int `json:"max_tokens"`
			Messages  []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.MaxTokens < 1 {
			http.Error(w, "stub: need a JSON body with max_tokens >= 1", http.StatusBadRequest)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "stub: response writer cannot flush", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(payload string) {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}

		send(`{"choices":[{"delta":{"role":"assistant","content":""}}]}`)
		for i := range req.MaxTokens {
			wait := o.Gap
			if i == 0 {
				wait = o.FirstDelay
			}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(wait):
			}
			send(`{"choices":[{"delta":{"content":"tok "}}]}`)
		}
		if !o.OmitUsage {
			words := 0
			for _, m := range req.Messages {
				words += len(strings.Fields(m.Content))
			}
			send(fmt.Sprintf(`{"choices":[],"usage":{"prompt_tokens":%d,"completion_tokens":%d}}`,
				words, req.MaxTokens))
		}
		send("[DONE]")
	})
	return mux
}

package client

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/saintparish4/tokbench/internal/stub"
)

const (
	roleChunk  = `data: {"choices":[{"delta":{"role":"assistant","content":""}}]}` + "\n\n"
	textChunk  = `data: {"choices":[{"delta":{"content":"hi"}}]}` + "\n\n"
	usageChunk = `data: {"choices":[],"usage":{"prompt_tokens":7,"completion_tokens":2}}` + "\n\n"
	done       = "data: [DONE]\n\n"
)

// tick returns a clock that advances 10ms on every call, so a test can
// assert exact gaps without sleeping.
func tick() func() time.Time {
	now := time.Unix(0, 0)
	return func() time.Time {
		now = now.Add(10 * time.Millisecond)
		return now
	}
}

func readStream(stream string) (Trace, error) {
	c := &Client{Now: tick()}
	var tr Trace
	err := c.read(strings.NewReader(stream), &tr)
	return tr, err
}

func TestReadRecordsOnlyChunksThatCarryText(t *testing.T) {
	tr, err := readStream(roleChunk + textChunk + textChunk + usageChunk + done)
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Chunks) != 2 {
		t.Fatalf("got %d text chunks, want 2 (role and usage chunks must not count)", len(tr.Chunks))
	}
	// Each event is a data line plus a blank line: two clock ticks apart.
	if gap := tr.Chunks[1].Sub(tr.Chunks[0]); gap != 20*time.Millisecond {
		t.Errorf("gap between text chunks = %v, want 20ms", gap)
	}
}

func TestReadCountsReasoningDeltasAsText(t *testing.T) {
	stream := `data: {"choices":[{"delta":{"reasoning_content":"hm"}}]}` + "\n\n" +
		`data: {"choices":[{"delta":{"reasoning":"hm"}}]}` + "\n\n" + done
	tr, err := readStream(stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Chunks) != 2 {
		t.Fatalf("got %d text chunks, want 2", len(tr.Chunks))
	}
}

func TestReadKeepsProviderUsageWhenPresent(t *testing.T) {
	tr, err := readStream(textChunk + usageChunk + done)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Usage == nil || *tr.Usage != (Usage{PromptTokens: 7, CompletionTokens: 2}) {
		t.Fatalf("usage = %+v, want prompt 7 and completion 2", tr.Usage)
	}

	tr, err = readStream(textChunk + done)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Usage != nil {
		t.Fatalf("usage = %+v, want nil when the stream sends none", tr.Usage)
	}
}

func TestReadIgnoresCommentsAndNullUsage(t *testing.T) {
	stream := ": keep-alive\n\n" +
		`data: {"choices":[{"delta":{"content":"hi"}}],"usage":null}` + "\n\n" + done
	tr, err := readStream(stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Chunks) != 1 || tr.Usage != nil {
		t.Fatalf("got %d chunks and usage %+v, want 1 chunk and nil usage", len(tr.Chunks), tr.Usage)
	}
}

func TestReadAcceptsDoneWithoutTrailingNewline(t *testing.T) {
	if _, err := readStream(textChunk + "data: [DONE]"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadFailsWhenStreamEndsBeforeDone(t *testing.T) {
	tr, err := readStream(roleChunk + textChunk)
	if !errors.Is(err, ErrTruncated) {
		t.Fatalf("err = %v, want ErrTruncated", err)
	}
	if len(tr.Chunks) != 1 {
		t.Errorf("got %d chunks, want the 1 seen before truncation", len(tr.Chunks))
	}
}

func TestReadFailsOnMalformedChunk(t *testing.T) {
	_, err := readStream("data: {not json\n\n" + done)
	if err == nil || errors.Is(err, ErrTruncated) {
		t.Fatalf("err = %v, want a decode error", err)
	}
}

func TestReadFailsOnErrorPayload(t *testing.T) {
	_, err := readStream(`data: {"error":{"message":"out of memory"}}` + "\n\n" + done)
	if err == nil || !strings.Contains(err.Error(), "out of memory") {
		t.Fatalf("err = %v, want the provider's message", err)
	}
}

func TestStreamSendsOpenAIRequestShape(t *testing.T) {
	type body struct {
		Model         string `json:"model"`
		MaxTokens     int    `json:"max_tokens"`
		Stream        bool   `json:"stream"`
		StreamOptions struct {
			IncludeUsage bool `json:"include_usage"`
		} `json:"stream_options"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	type seen struct {
		path, auth string
		body       body
	}
	got := make(chan seen, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := seen{path: r.URL.Path, auth: r.Header.Get("Authorization")}
		if err := json.NewDecoder(r.Body).Decode(&s.body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		got <- s
		_, _ = io.WriteString(w, textChunk+usageChunk+done)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL + "/v1", APIKey: "secret", Model: "m", MaxTokens: 9, HTTP: srv.Client(), Now: time.Now}
	if _, err := c.Stream(t.Context(), "hello"); err != nil {
		t.Fatal(err)
	}
	s := <-got
	if s.path != "/v1/chat/completions" {
		t.Errorf("path = %q", s.path)
	}
	if s.auth != "Bearer secret" {
		t.Errorf("Authorization = %q", s.auth)
	}
	b := s.body
	if b.Model != "m" || b.MaxTokens != 9 || !b.Stream || !b.StreamOptions.IncludeUsage {
		t.Errorf("request body = %+v", b)
	}
	if len(b.Messages) != 1 || b.Messages[0].Role != "user" || b.Messages[0].Content != "hello" {
		t.Errorf("messages = %+v", b.Messages)
	}
}

func TestStreamReportsNon200WithBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "model not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL + "/v1", Model: "m", MaxTokens: 1, HTTP: srv.Client(), Now: time.Now}
	_, err := c.Stream(t.Context(), "hello")
	if err == nil || !strings.Contains(err.Error(), "status 404") || !strings.Contains(err.Error(), "model not found") {
		t.Fatalf("err = %v, want status and body", err)
	}
}

func TestStreamAgainstStubServer(t *testing.T) {
	srv := httptest.NewServer(stub.Handler(stub.Options{}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL + "/v1", Model: "stub", MaxTokens: 5, HTTP: srv.Client(), Now: time.Now}
	tr, err := c.Stream(t.Context(), "hello there")
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Chunks) != 5 {
		t.Fatalf("got %d text chunks, want 5", len(tr.Chunks))
	}
	if tr.Usage == nil || *tr.Usage != (Usage{PromptTokens: 2, CompletionTokens: 5}) {
		t.Fatalf("usage = %+v, want prompt 2 and completion 5", tr.Usage)
	}
	if tr.End.Before(tr.Chunks[4]) || tr.Chunks[0].Before(tr.Start) {
		t.Errorf("timeline out of order: start %v, chunks %v, end %v", tr.Start, tr.Chunks, tr.End)
	}
}

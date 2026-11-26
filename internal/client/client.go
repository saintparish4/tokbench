// Package client sends one streaming chat-completions request and records
// when each piece of the answer arrived.
//
// It deliberately returns raw observations (a Trace), not metrics. The rules
// for what counts as a token arrival live here; the arithmetic lives in
// package metrics. Three rules are fixed by directive/b1.md section 0:
//
//   - A chunk counts as text only if its delta carries non-empty content or
//     reasoning text. The role-only opening chunk and the usage chunk do not.
//   - A chunk's timestamp is taken as soon as its line is read, before the
//     JSON is decoded, so decode time is not charged to the server.
//   - A stream that ends without "data: [DONE]" is an error. A truncated
//     answer must never be mistaken for a short one.
//
// Nothing here retries. A benchmark that hides failures reports a faster
// system than the one that exists.
package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrTruncated means the response body ended before the [DONE] sentinel.
var ErrTruncated = errors.New("stream ended before [DONE]")

// Usage is the provider's own token count for one request.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// Trace is the raw timeline of one streamed request.
type Trace struct {
	Start  time.Time   // just before the request is sent
	Chunks []time.Time // arrival of each chunk that carried text
	End    time.Time   // when the stream finished or failed
	Usage  *Usage      // nil when the provider sent no usage
}

// Client talks to one OpenAI-compatible endpoint with one model.
type Client struct {
	BaseURL   string // ends in /v1, no trailing slash
	APIKey    string // empty means no Authorization header
	Model     string
	MaxTokens int
	Seed      int64
	HTTP      *http.Client
	Now       func() time.Time
}

// NewHTTPClient returns a client that keeps maxConns idle connections open
// and never asks for compression, which would buffer the event stream.
func NewHTTPClient(maxConns int) *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = maxConns
	t.MaxIdleConnsPerHost = maxConns
	t.DisableCompression = true
	return &http.Client{Transport: t}
}

type request struct {
	Model         string        `json:"model"`
	Messages      []message     `json:"messages"`
	MaxTokens     int           `json:"max_tokens"`
	Temperature   float64       `json:"temperature"`
	Seed          int64         `json:"seed"`
	Stream        bool          `json:"stream"`
	StreamOptions streamOptions `json:"stream_options"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			Reasoning        string `json:"reasoning"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *Usage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c chunk) hasText() bool {
	for _, choice := range c.Choices {
		d := choice.Delta
		if d.Content != "" || d.ReasoningContent != "" || d.Reasoning != "" {
			return true
		}
	}
	return false
}

// Stream sends prompt as a single user message and reads the whole stream.
// On error the returned Trace holds whatever was observed before the failure.
func (c *Client) Stream(ctx context.Context, prompt string) (Trace, error) {
	body, err := json.Marshal(request{
		Model:         c.Model,
		Messages:      []message{{Role: "user", Content: prompt}},
		MaxTokens:     c.MaxTokens,
		Seed:          c.Seed,
		Stream:        true,
		StreamOptions: streamOptions{IncludeUsage: true},
	})
	if err != nil {
		return Trace{}, fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Trace{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	tr := Trace{Start: c.Now()}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		tr.End = c.Now()
		return tr, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		tr.End = c.Now()
		return tr, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	err = c.read(resp.Body, &tr)
	tr.End = c.Now()
	return tr, err
}

// read consumes the event stream line by line, appending to tr.
func (c *Client) read(body io.Reader, tr *Trace) error {
	r := bufio.NewReader(body)
	for {
		line, err := r.ReadBytes('\n')
		now := c.Now()
		if data, ok := bytes.CutPrefix(bytes.TrimSpace(line), []byte("data:")); ok {
			data = bytes.TrimSpace(data)
			if bytes.Equal(data, []byte("[DONE]")) {
				return nil
			}
			var ch chunk
			if jerr := json.Unmarshal(data, &ch); jerr != nil {
				return fmt.Errorf("decode chunk %q: %w", clip(data), jerr)
			}
			if ch.Error != nil {
				return fmt.Errorf("stream error: %s", ch.Error.Message)
			}
			if ch.Usage != nil {
				tr.Usage = ch.Usage
			}
			if ch.hasText() {
				tr.Chunks = append(tr.Chunks, now)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return ErrTruncated
			}
			return fmt.Errorf("read stream: %w", err)
		}
	}
}

// clip keeps error messages short when a server sends a huge bad line.
func clip(b []byte) []byte {
	if len(b) > 120 {
		return b[:120]
	}
	return b
}

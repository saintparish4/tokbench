// Package metrics turns one request's Trace into the numbers tokbench
// reports: time to first token, inter-token gaps and tokens per second.
//
// It is pure arithmetic on timestamps the client already recorded; nothing
// here reads a clock or the network. The token count has exactly two possible
// sources and every Sample says which one it used, because directive/
// buildplan.md section 4 forbids silently mixing provider counts with
// anything else:
//
//   - FromUsage: the provider's usage.completion_tokens.
//   - FromChunks: the number of text chunks, used only when usage is absent
//     or zero. Servers may pack several tokens into a chunk, so this is a
//     lower bound and reports must flag it.
package metrics

import (
	"errors"
	"time"

	"github.com/saintparish4/tokbench/internal/client"
)

// TokenSource names where a Sample's token count came from.
type TokenSource string

// The two token sources. See the package comment.
const (
	FromUsage  TokenSource = "usage"
	FromChunks TokenSource = "chunks"
)

// ErrNoText means the stream completed but never carried a text chunk, so
// there is no first token to time.
var ErrNoText = errors.New("stream carried no text chunks")

// Sample is one request's measurements.
type Sample struct {
	TTFT         time.Duration   // request sent to first text chunk
	Gaps         []time.Duration // between successive text chunks
	Total        time.Duration   // request sent to end of stream
	PromptTokens int             // from usage; 0 when the source is chunks
	OutputTokens int
	Chunks       int // text chunks seen, whatever the token source
	Source       TokenSource
	TokPerSec    float64 // OutputTokens / Total
}

// FromTrace computes a Sample. It fails only when there is no text to time.
func FromTrace(tr client.Trace) (Sample, error) {
	if len(tr.Chunks) == 0 {
		return Sample{}, ErrNoText
	}
	s := Sample{
		TTFT:         tr.Chunks[0].Sub(tr.Start),
		Total:        tr.End.Sub(tr.Start),
		OutputTokens: len(tr.Chunks),
		Chunks:       len(tr.Chunks),
		Source:       FromChunks,
	}
	for i := 1; i < len(tr.Chunks); i++ {
		s.Gaps = append(s.Gaps, tr.Chunks[i].Sub(tr.Chunks[i-1]))
	}
	if tr.Usage != nil && tr.Usage.CompletionTokens > 0 {
		s.PromptTokens = tr.Usage.PromptTokens
		s.OutputTokens = tr.Usage.CompletionTokens
		s.Source = FromUsage
	}
	if sec := s.Total.Seconds(); sec > 0 {
		s.TokPerSec = float64(s.OutputTokens) / sec
	}
	return s, nil
}
package metrics

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/saintparish4/tokbench/internal/client"
)

func at(ms int) time.Time {
	return time.Unix(0, 0).Add(time.Duration(ms) * time.Millisecond)
}

func TestFromTraceMeasuresTTFTGapsAndRate(t *testing.T) {
	s, err := FromTrace(client.Trace{
		Start:  at(0),
		Chunks: []time.Time{at(200), at(230), at(270)},
		End:    at(500),
		Usage:  &client.Usage{PromptTokens: 40, CompletionTokens: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.TTFT != 200*time.Millisecond {
		t.Errorf("TTFT = %v, want 200ms", s.TTFT)
	}
	want := []time.Duration{30 * time.Millisecond, 40 * time.Millisecond}
	if !slices.Equal(s.Gaps, want) {
		t.Errorf("Gaps = %v, want %v", s.Gaps, want)
	}
	if s.Total != 500*time.Millisecond {
		t.Errorf("Total = %v, want 500ms", s.Total)
	}
	if s.OutputTokens != 5 || s.PromptTokens != 40 || s.Chunks != 3 || s.Source != FromUsage {
		t.Errorf("tokens: %+v", s)
	}
	if s.TokPerSec != 10 {
		t.Errorf("TokPerSec = %v, want 10 (5 tokens in 0.5s)", s.TokPerSec)
	}
}

func TestFromTraceFallsBackToChunkCountWithoutUsage(t *testing.T) {
	s, err := FromTrace(client.Trace{Start: at(0), Chunks: []time.Time{at(10), at(20), at(30)}, End: at(40)})
	if err != nil {
		t.Fatal(err)
	}
	if s.OutputTokens != 3 || s.Source != FromChunks || s.PromptTokens != 0 {
		t.Errorf("got %+v, want 3 output tokens labelled chunks", s)
	}
}

func TestFromTraceTreatsZeroUsageAsMissing(t *testing.T) {
	s, err := FromTrace(client.Trace{
		Start: at(0), Chunks: []time.Time{at(10), at(20)}, End: at(30),
		Usage: &client.Usage{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.OutputTokens != 2 || s.Source != FromChunks {
		t.Errorf("got %+v, want the chunk-count fallback", s)
	}
}

func TestFromTraceSingleChunkHasNoGaps(t *testing.T) {
	s, err := FromTrace(client.Trace{Start: at(0), Chunks: []time.Time{at(10)}, End: at(20)})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Gaps) != 0 {
		t.Errorf("Gaps = %v, want none", s.Gaps)
	}
}

func TestFromTraceRejectsStreamWithoutText(t *testing.T) {
	_, err := FromTrace(client.Trace{Start: at(0), End: at(10), Usage: &client.Usage{CompletionTokens: 4}})
	if !errors.Is(err, ErrNoText) {
		t.Fatalf("err = %v, want ErrNoText", err)
	}
}
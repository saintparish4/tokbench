// Command tokbench benchmarks an OpenAI-compatible chat-completions endpoint.
//
// At milestone B1 it sends a single streaming request and prints time to
// first token, inter-token latency and tokens per second for it. The sweep
// flags are accepted and validated but only the first prompt is sent;
// milestone B2 replaces bench with the concurrency runner and the report.
//
// All logic lives in run, which takes its arguments, environment and writers
// as parameters so the tests can drive the whole CLI in-process.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/saintparish4/tokbench/internal/client"
	"github.com/saintparish4/tokbench/internal/config"
	"github.com/saintparish4/tokbench/internal/metrics"
	"github.com/saintparish4/tokbench/internal/prompt"
)

// version is overwritten at release time with -ldflags "-X main.version=...".
var version = "dev"

const usage = "usage: tokbench <run|version> [flags]; see tokbench run -h"

func main() {
	os.Exit(realMain())
}

func realMain() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	err := run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr)
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return 0
	}
	fmt.Fprintln(os.Stderr, "tokbench:", err)
	return 1
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	switch args[0] {
	case "version":
		_, err := fmt.Fprintln(stdout, version)
		return err
	case "run":
		return bench(ctx, args[1:], getenv, stdout, stderr)
	default:
		return fmt.Errorf("unknown command %q; %s", args[0], usage)
	}
}

func bench(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) error {
	cfg, err := config.Parse(args, getenv, stderr)
	if err != nil {
		return err
	}
	prompts, err := prompt.Set(uint64(cfg.Seed), cfg.Prompts, cfg.PromptTokens)
	if err != nil {
		return err
	}
	c := &client.Client{
		BaseURL:   cfg.BaseURL,
		APIKey:    cfg.APIKey,
		Model:     cfg.Model,
		MaxTokens: cfg.GenTokens,
		Seed:      cfg.Seed,
		HTTP:      client.NewHTTPClient(1),
		Now:       time.Now,
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	tr, err := c.Stream(ctx, prompts[0])
	if err != nil {
		return err
	}
	s, err := metrics.FromTrace(tr)
	if err != nil {
		return err
	}
	return printSample(stdout, s)
}

func printSample(w io.Writer, s metrics.Sample) error {
	var meanGap time.Duration
	for _, g := range s.Gaps {
		meanGap += g
	}
	if len(s.Gaps) > 0 {
		meanGap /= time.Duration(len(s.Gaps))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%-14s %s\n", "ttft", ms(s.TTFT))
	fmt.Fprintf(&b, "%-14s %s mean over %d gaps\n", "inter-token", ms(meanGap), len(s.Gaps))
	fmt.Fprintf(&b, "%-14s %s\n", "total", ms(s.Total))
	fmt.Fprintf(&b, "%-14s %d (source: %s)\n", "output tokens", s.OutputTokens, s.Source)
	fmt.Fprintf(&b, "%-14s %d\n", "prompt tokens", s.PromptTokens)
	fmt.Fprintf(&b, "%-14s %.1f\n", "tok/s", s.TokPerSec)
	_, err := io.WriteString(w, b.String())
	return err
}

func ms(d time.Duration) string {
	return fmt.Sprintf("%.1f ms", float64(d)/float64(time.Millisecond))
}

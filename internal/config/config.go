// Package config owns the `tokbench run` flag set: names, defaults and the
// checks that reject a run before it spends anything.
//
// The eight flags proposed in directive/buildplan.md section 9 keep their
// names. The rest exist because a sweep needs a stopping rule (--requests,
// --duration), a fair start (--warmup), a failure bound (--timeout), a
// reproducible prompt set (--prompts, --seed) and manifest fields the tool
// cannot discover (--gpu-type, --label).
//
// The API key is the one value that must never be printed. It is not a flag
// default, so -h cannot show it, and Host is the only accessor a report
// should use for the endpoint.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Config is a validated `tokbench run` invocation.
type Config struct {
	BaseURL      string
	APIKey       string
	Model        string
	Concurrency  []int
	Requests     int
	Duration     time.Duration
	Warmup       int
	Timeout      time.Duration
	PromptTokens int
	GenTokens    int
	Prompts      int
	Seed         int64
	GPUHourlyUSD float64
	GPUType      string
	Label        string
	Format       string
	Out          string
}

// Parse reads args (without the program or subcommand name). Usage and flag
// errors are written to errOut. It returns flag.ErrHelp for -h.
func Parse(args []string, getenv func(string) string, errOut io.Writer) (Config, error) {
	var (
		c      Config
		levels string
	)
	fs := flag.NewFlagSet("tokbench run", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.StringVar(&c.BaseURL, "base-url", "", "OpenAI-compatible base URL ending in /v1 (required)")
	fs.StringVar(&c.APIKey, "api-key", "", "bearer token; falls back to $TOKBENCH_API_KEY")
	fs.StringVar(&c.Model, "model", "", "model name sent in each request (required)")
	fs.StringVar(&levels, "concurrency", "1", "comma-separated concurrency levels, e.g. 1,2,4,8,16")
	fs.IntVar(&c.Requests, "requests", 0, "measured requests per level; 0 means no cap")
	fs.DurationVar(&c.Duration, "duration", time.Minute, "time per level; 0 means no limit")
	fs.IntVar(&c.Warmup, "warmup", 1, "unmeasured requests per worker before each level")
	fs.DurationVar(&c.Timeout, "timeout", 2*time.Minute, "per-request timeout")
	fs.IntVar(&c.PromptTokens, "prompt-tokens", 512, "target prompt length in words, about one token each")
	fs.IntVar(&c.GenTokens, "gen-tokens", 256, "max_tokens for each completion")
	fs.IntVar(&c.Prompts, "prompts", 16, "number of distinct prompts in the set")
	fs.Int64Var(&c.Seed, "seed", 1, "seed for the prompt set and for each request")
	fs.Float64Var(&c.GPUHourlyUSD, "gpu-hourly-usd", 0, "GPU price per hour; 0 leaves the cost column empty")
	fs.StringVar(&c.GPUType, "gpu-type", "", "GPU type recorded in the manifest, e.g. H100")
	fs.StringVar(&c.Label, "label", "", "run label recorded in the manifest, e.g. lambda-h100")
	fs.StringVar(&c.Format, "format", "table", "stdout format: table or json")
	fs.StringVar(&c.Out, "out", "", "also write the JSON report to this file")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if fs.NArg() > 0 {
		return Config{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	if c.APIKey == "" {
		c.APIKey = getenv("TOKBENCH_API_KEY")
	}
	parsed, err := ParseLevels(levels)
	if err != nil {
		return Config{}, err
	}
	c.Concurrency = parsed
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")
	if err = c.validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// ParseLevels turns "1,2,4" into [1 2 4], keeping the order given.
func ParseLevels(s string) ([]int, error) {
	var out []int
	for _, field := range strings.Split(s, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil || n < 1 {
			return nil, fmt.Errorf("--concurrency %q: each level must be a whole number >= 1", s)
		}
		out = append(out, n)
	}
	return out, nil
}

// Host is the endpoint's host[:port], the only part of the base URL that
// belongs in a manifest.
func (c Config) Host() string {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return ""
	}
	return u.Host
}

func (c Config) validate() error {
	u, err := url.Parse(c.BaseURL)
	switch {
	case c.BaseURL == "":
		return errors.New("--base-url is required")
	case err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https"):
		return fmt.Errorf("--base-url %q must look like http(s)://host[:port]/v1", c.BaseURL)
	case c.Model == "":
		return errors.New("--model is required")
	case c.Requests < 0 || c.Duration < 0:
		return errors.New("--requests and --duration cannot be negative")
	case c.Requests == 0 && c.Duration == 0:
		return errors.New("set --requests or --duration; with both zero a level never ends")
	case c.Warmup < 0:
		return errors.New("--warmup cannot be negative")
	case c.Timeout <= 0:
		return errors.New("--timeout must be positive")
	case c.PromptTokens < 1 || c.GenTokens < 1 || c.Prompts < 1:
		return errors.New("--prompt-tokens, --gen-tokens and --prompts must be >= 1")
	case c.GPUHourlyUSD < 0:
		return errors.New("--gpu-hourly-usd cannot be negative")
	case c.Format != "table" && c.Format != "json":
		return fmt.Errorf("--format %q: want table or json", c.Format)
	}
	return nil
}
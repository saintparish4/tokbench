package config

import (
	"io"
	"slices"
	"testing"
	"time"
)

var base = []string{"--base-url", "http://127.0.0.1:8080/v1", "--model", "m"}

func parse(env map[string]string, args ...string) (Config, error) {
	return Parse(args, func(k string) string { return env[k] }, io.Discard)
}

func TestParseAppliesDefaults(t *testing.T) {
	c, err := parse(nil, base...)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.Concurrency, []int{1}) || c.Duration != time.Minute || c.Warmup != 1 {
		t.Errorf("sweep defaults: %+v", c)
	}
	if c.PromptTokens != 512 || c.GenTokens != 256 || c.Prompts != 16 || c.Seed != 1 {
		t.Errorf("prompt defaults: %+v", c)
	}
	if c.Format != "table" || c.Host() != "127.0.0.1:8080" {
		t.Errorf("format %q, host %q", c.Format, c.Host())
	}
}

func TestParseReadsTheProposedInterface(t *testing.T) {
	c, err := parse(nil,
		"--base-url", "https://api.example.com/v1/",
		"--api-key", "k",
		"--model", "m",
		"--concurrency", "1,2,4,8,16",
		"--prompt-tokens", "512",
		"--gen-tokens", "256",
		"--gpu-hourly-usd", "4.59",
		"--format", "json",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.Concurrency, []int{1, 2, 4, 8, 16}) {
		t.Errorf("Concurrency = %v", c.Concurrency)
	}
	if c.BaseURL != "https://api.example.com/v1" {
		t.Errorf("BaseURL = %q, want the trailing slash removed", c.BaseURL)
	}
	if c.APIKey != "k" || c.GPUHourlyUSD != 4.59 || c.Format != "json" {
		t.Errorf("got %+v", c)
	}
}

func TestParseAPIKeyFallsBackToEnvironment(t *testing.T) {
	env := map[string]string{"TOKBENCH_API_KEY": "from-env"}
	c, err := parse(env, base...)
	if err != nil {
		t.Fatal(err)
	}
	if c.APIKey != "from-env" {
		t.Errorf("APIKey = %q, want the environment value", c.APIKey)
	}
	c, err = parse(env, slices.Concat(base, []string{"--api-key", "from-flag"})...)
	if err != nil {
		t.Fatal(err)
	}
	if c.APIKey != "from-flag" {
		t.Errorf("APIKey = %q, want the flag to win", c.APIKey)
	}
}

func TestParseRejectsInvalidInput(t *testing.T) {
	cases := map[string][]string{
		"missing base url":        {"--model", "m"},
		"missing model":           {"--base-url", "http://h/v1"},
		"base url without scheme": {"--base-url", "localhost:8080/v1", "--model", "m"},
		"zero level":              slices.Concat(base, []string{"--concurrency", "0"}),
		"non-numeric level":       slices.Concat(base, []string{"--concurrency", "1,x"}),
		"level never ends":        slices.Concat(base, []string{"--requests", "0", "--duration", "0"}),
		"unknown format":          slices.Concat(base, []string{"--format", "yaml"}),
		"zero gen tokens":         slices.Concat(base, []string{"--gen-tokens", "0"}),
		"negative price":          slices.Concat(base, []string{"--gpu-hourly-usd", "-1"}),
		"stray argument":          slices.Concat(base, []string{"extra"}),
		"unknown flag":            slices.Concat(base, []string{"--nope"}),
	}
	for name, args := range cases {
		if _, err := parse(nil, args...); err == nil {
			t.Errorf("%s: Parse returned no error", name)
		}
	}
}
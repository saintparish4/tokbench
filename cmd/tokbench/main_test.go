package main

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/saintparish4/tokbench/internal/stub"
)

func runCLI(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	err = run(t.Context(), args, func(string) string { return "" }, &out, &errOut)
	return out.String(), errOut.String(), err
}

func TestRunSingleStreamPrintsMetrics(t *testing.T) {
	srv := httptest.NewServer(stub.Handler(stub.Options{}))
	defer srv.Close()

	out, _, err := runCLI(t, "run", "--base-url", srv.URL+"/v1", "--model", "stub",
		"--gen-tokens", "8", "--prompt-tokens", "16")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ttft", "inter-token", "8 (source: usage)", "tok/s"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
}

func TestRunLabelsChunkCountFallback(t *testing.T) {
	srv := httptest.NewServer(stub.Handler(stub.Options{OmitUsage: true}))
	defer srv.Close()

	out, _, err := runCLI(t, "run", "--base-url", srv.URL+"/v1", "--model", "stub", "--gen-tokens", "8")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "8 (source: chunks)") {
		t.Errorf("fallback is not labelled:\n%s", out)
	}
}

func TestVersionPrintsVersion(t *testing.T) {
	out, _, err := runCLI(t, "version")
	if err != nil {
		t.Fatal(err)
	}
	if out != "dev\n" {
		t.Errorf("version output = %q, want %q", out, "dev\n")
	}
}

func TestRunRejectsMissingAndUnknownCommands(t *testing.T) {
	if _, _, err := runCLI(t); err == nil {
		t.Error("no arguments: run returned no error")
	}
	if _, _, err := runCLI(t, "frobnicate"); err == nil {
		t.Error("unknown command: run returned no error")
	}
}

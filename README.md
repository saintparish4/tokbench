# tokbench

`tokbench` is a small Go CLI that measures how fast an OpenAI-compatible
chat-completions endpoint streams: time to first token, inter-token latency
and tokens per second. It builds to one static binary and has no third-party
dependencies.

The goal is a tool that answers two questions about any such endpoint: how
fast is it at the tail under load, and what does a million tokens cost on the
GPU serving it?

## Status

Early. The measurement core works for a single request. The load sweep and
reporting are not built yet.

| Piece | State |
|---|---|
| Streaming client, TTFT, inter-token latency, tok/s for one request | Works |
| Seeded, reproducible prompt set | Works |
| Stub endpoint for tests and local runs | Works |
| Concurrency sweep with p50/p95/p99 per level | Planned |
| Cost per 1M output tokens from a GPU hourly price | Planned |
| JSON report with a run manifest, and a table | Planned |
| Side-by-side comparison of two runs | Planned |
| Release binaries and CI | Planned |

There are no benchmark results in this repository yet. None will be added
until they come from a real run with its manifest committed beside them.

## Quick start

Needs Go 1.25 or newer.

```bash
git clone https://github.com/saintparish4/tokbench.git
cd tokbench
go build -o tokbench ./cmd/tokbench
```

Try it against the built-in stub, which needs no model and no GPU:

```bash
go run ./cmd/stubserver &
./tokbench run --base-url http://127.0.0.1:8080/v1 --model stub --gen-tokens 32
kill %1
```

Against a real endpoint, pass its base URL (ending in `/v1`) and a model name
it serves:

```bash
export TOKBENCH_API_KEY=your-key        # omit for servers without auth
./tokbench run --base-url https://your-endpoint.example/v1 --model your-model
```

It sends one streaming request and prints six lines:

| Line | Meaning |
|---|---|
| `ttft` | Time from sending the request to the first chunk that carries text |
| `inter-token` | Mean gap between successive text chunks, and how many gaps |
| `total` | Time from sending the request to the end of the stream |
| `output tokens` | Tokens generated, and where the count came from |
| `prompt tokens` | Prompt tokens as reported by the provider |
| `tok/s` | Output tokens divided by total time |

## How it measures

**Time to first token** ends at the first stream chunk whose delta has
non-empty `content`, `reasoning_content` or `reasoning`. OpenAI-style streams
open with a role-only chunk that carries no text; counting it would report
the server's header flush as a token.

**Inter-token latency** is the gap between successive text-bearing chunks.
Each chunk is timestamped when its line is read, before the JSON is decoded.
A server may put more than one token in a chunk, so this is a per-chunk
figure. Comparing `output tokens` with the gap count shows when that is
happening.

**Token counts** come from the provider. Requests set
`stream_options.include_usage`, and the count is `usage.completion_tokens`
from the stream. If the stream carries no usage, `tokbench` counts text
chunks instead and prints `(source: chunks)` rather than `(source: usage)`,
so the two are never mixed silently.

**Prompts** are generated from a seed: a fixed instruction followed by
`--prompt-tokens` common short words. The same seed always gives the same
prompts. The length is a word-count target, not an exact token count; the
provider's own count is what gets printed.

**Failures are not retried.** A stream that ends without `data: [DONE]` is an
error, so a truncated response cannot pass as a short one.

## Flags

`tokbench run -h` prints the full list. The ones that have an effect today:

| Flag | Default | Meaning |
|---|---|---|
| `--base-url` | required | Base URL ending in `/v1` |
| `--model` | required | Model name sent in the request |
| `--api-key` | `$TOKBENCH_API_KEY` | Bearer token, sent only as the `Authorization` header |
| `--prompt-tokens` | 512 | Target prompt length in words |
| `--gen-tokens` | 256 | `max_tokens` for the completion |
| `--seed` | 1 | Seed for the prompt set and the request |
| `--timeout` | 2m | Per-request timeout |

`--concurrency`, `--requests`, `--duration`, `--warmup`, `--prompts`,
`--gpu-hourly-usd`, `--gpu-type`, `--label`, `--format` and `--out` are
accepted and validated now so the interface will not change, but they do
nothing until the sweep and report land.

`tokbench version` prints the version.

## The stub server

`cmd/stubserver` is a fake endpoint for development. It streams one text
chunk per requested token with delays you choose, so you can check that the
numbers `tokbench` prints match the numbers you configured.

| Flag | Default | Meaning |
|---|---|---|
| `-addr` | `127.0.0.1:8080` | Listen address |
| `-first` | 200ms | Delay before the first text chunk |
| `-gap` | 20ms | Delay between later text chunks |
| `-omit-usage` | false | Leave out the usage chunk, to exercise the fallback |

Numbers measured against the stub describe those flags, not any real system.

## Layout

```
cmd/tokbench      the CLI
cmd/stubserver    the fake endpoint, as a command
internal/config   flags and validation
internal/prompt   the seeded prompt set
internal/client   the streaming request and its raw timeline
internal/metrics  timeline to TTFT, gaps and tok/s
internal/stub     the fake endpoint, as an http.Handler
```

## Development

```bash
go test -race ./...
golangci-lint run ./...
```

Every test runs against the in-process stub. Nothing needs a GPU, a model or
a network.

## Limits

- OpenAI-compatible chat completions only. No other protocols and no
  provider SDKs.
- It measures speed, not output quality.
- One request at a time, for now.

// Package prompt builds the shared prompt set that every run sends.
//
// A two-cloud comparison is only fair if both endpoints see the same
// requests, so Set is a pure function of (seed, n, targetTokens): no clock,
// no hostname, no map iteration. Hash fingerprints a set so two manifests can
// prove they used the same one.
//
// Length is a target, not a promise. Each prompt is targetTokens common short
// words, which most tokenizers encode as about one token each. The provider's
// usage.prompt_tokens is the measured value, and that is what reports print.
package prompt

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"strings"
)

// instruction asks for open-ended output so the model runs to max_tokens
// instead of stopping early, which would make output lengths differ by run.
const instruction = "Write one continuous story that uses every word below, in order. " +
	"Do not stop, summarize or add a title; keep writing until you are cut off.\n\n"

var words = []string{
	"time", "year", "day", "man", "world", "life", "hand", "part",
	"child", "eye", "place", "work", "week", "case", "point", "home",
	"water", "room", "night", "city", "story", "book", "word", "side",
	"kind", "head", "house", "friend", "hour", "game", "line", "car",
}

// Set returns n prompts, each the instruction followed by TargetTokens words
// drawn with the given seed
func Set(seed uint64, n, targetTokens int) ([]string, error) {
	if n < 1 || targetTokens < 1 {
		return nil, fmt.Errorf("prompt: need n >= 1 and targetTokens >= 1, got %d and %d", n,
			targetTokens)
	}
	rng := rand.New(rand.NewPCG(seed, 0))
	out := make([]string, n)
	for i := range out {
		var b strings.Builder
		b.WriteString(instruction)
		for w := range targetTokens {
			if w > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(words[rng.IntN(len(words))])
		}
		out[i] = b.String()
	}
	return out, nil
}

// Hash returns the SHA-256 of the set. prompts are joined with a NUL byte so
// that moving text across a prompt boundary changes the hash
func Hash(prompts []string) string {
	sum := sha256.Sum256([]byte(strings.Join(prompts, "\x00")))
	return hex.EncodeToString(sum[:])
}

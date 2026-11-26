package prompt

import (
	"slices"
	"strings"
	"testing"
)

func TestSetIsDeterministicForASeed(t *testing.T) {
	a, err := Set(7, 4, 64)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Set(7, 4, 64)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a, b) {
		t.Fatal("same seed produced different prompt sets")
	}
}

func TestSetChangesWithSeed(t *testing.T) {
	a, err := Set(1, 4, 64)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Set(2, 4, 64)
	if err != nil {
		t.Fatal(err)
	}
	if Hash(a) == Hash(b) {
		t.Fatal("different seeds produced the same prompt set")
	}
}

func TestSetWordCountMatchesTarget(t *testing.T) {
	prompts, err := Set(1, 3, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(prompts) != 3 {
		t.Fatalf("got %d prompts, want 3", len(prompts))
	}
	for i, p := range prompts {
		body, ok := strings.CutPrefix(p, instruction)
		if !ok {
			t.Fatalf("prompt %d does not start with the instruction", i)
		}
		if got := len(strings.Fields(body)); got != 100 {
			t.Errorf("prompt %d has %d words, want 100", i, got)
		}
	}
}

func TestSetPromptsDifferWithinASet(t *testing.T) {
	prompts, err := Set(1, 2, 64)
	if err != nil {
		t.Fatal(err)
	}
	if prompts[0] == prompts[1] {
		t.Fatal("two prompts in one set are identical")
	}
}

func TestSetRejectsNonPositiveArguments(t *testing.T) {
	for _, tc := range []struct{ n, tokens int }{{0, 10}, {1, 0}, {-1, 10}} {
		if _, err := Set(1, tc.n, tc.tokens); err == nil {
			t.Errorf("Set(1, %d, %d) returned no error", tc.n, tc.tokens)
		}
	}
}

func TestHashSeparatesPromptBoundaries(t *testing.T) {
	if Hash([]string{"ab", "c"}) == Hash([]string{"a", "bc"}) {
		t.Fatal("hash ignores where one prompt ends and the next begins")
	}
}

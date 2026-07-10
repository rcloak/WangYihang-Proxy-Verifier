package util_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WangYihang/Proxy-Verifier/internal/util"
)

func TestDeduplicateLinesRandomly(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.txt")
	out := filepath.Join(dir, "out.txt")

	if err := os.WriteFile(in, []byte("a\nb\na\nc\nb\nc\nc\n"), 0644); err != nil {
		t.Fatal(err)
	}

	n, err := util.DeduplicateLinesRandomly(in, out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 3 {
		t.Errorf("unique count = %d, want 3", n)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, line := range strings.Fields(string(data)) {
		got[line] = true
	}
	if len(got) != 3 {
		t.Errorf("output has %d unique lines, want 3 (%q)", len(got), string(data))
	}
	for _, want := range []string{"a", "b", "c"} {
		if !got[want] {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestDeduplicateLinesRandomlyMissingInput(t *testing.T) {
	dir := t.TempDir()
	_, err := util.DeduplicateLinesRandomly(filepath.Join(dir, "does-not-exist.txt"), filepath.Join(dir, "out.txt"))
	if err == nil {
		t.Error("expected an error for a missing input file, got nil")
	}
}

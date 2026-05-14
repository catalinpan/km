package grep

import (
	"reflect"
	"strings"
	"testing"
)

func runStream(t *testing.T, f *Filter, in []string) []string {
	t.Helper()
	var got []string
	for _, line := range in {
		got = append(got, f.Apply(line)...)
	}
	return got
}

func TestParse_PatternOnly(t *testing.T) {
	f, err := Parse("Events:")
	if err != nil {
		t.Fatal(err)
	}
	out := runStream(t, f, []string{"name: foo", "Events: bar", "Source: baz"})
	want := []string{"Events: bar"}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("got %v, want %v", out, want)
	}
}

func TestParse_AfterContext(t *testing.T) {
	f, err := Parse("Events: -A 2")
	if err != nil {
		t.Fatal(err)
	}
	in := []string{"a", "b", "Events: c", "d", "e", "f", "g"}
	want := []string{"Events: c", "d", "e"}
	out := runStream(t, f, in)
	if !reflect.DeepEqual(out, want) {
		t.Errorf("got %v, want %v", out, want)
	}
}

func TestParse_BeforeContext(t *testing.T) {
	f, err := Parse("hit -B 2")
	if err != nil {
		t.Fatal(err)
	}
	in := []string{"a", "b", "c", "hit", "d"}
	want := []string{"b", "c", "hit"}
	out := runStream(t, f, in)
	if !reflect.DeepEqual(out, want) {
		t.Errorf("got %v, want %v", out, want)
	}
}

func TestParse_Context(t *testing.T) {
	f, err := Parse("hit -C 1")
	if err != nil {
		t.Fatal(err)
	}
	in := []string{"a", "b", "hit", "c", "d"}
	want := []string{"b", "hit", "c"}
	out := runStream(t, f, in)
	if !reflect.DeepEqual(out, want) {
		t.Errorf("got %v, want %v", out, want)
	}
}

func TestParse_IgnoreCase(t *testing.T) {
	f, err := Parse("EVENTS -i")
	if err != nil {
		t.Fatal(err)
	}
	out := runStream(t, f, []string{"events foo", "Other"})
	want := []string{"events foo"}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("got %v, want %v", out, want)
	}
}

func TestParse_Invert(t *testing.T) {
	f, err := Parse("skip -v")
	if err != nil {
		t.Fatal(err)
	}
	out := runStream(t, f, []string{"skip me", "keep me", "skip again", "and keep"})
	want := []string{"keep me", "and keep"}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("got %v, want %v", out, want)
	}
}

func TestParse_DashANumber(t *testing.T) {
	// Common shorthand: -A50 with no space.
	f, err := Parse("hit -A50")
	if err != nil {
		t.Fatal(err)
	}
	out := runStream(t, f, []string{"hit", "a", "b"})
	if len(out) != 3 {
		t.Errorf("expected 3 lines (hit + 2 of -A), got %d: %v", len(out), out)
	}
}

func TestApply_MatchesAgainstStrippedANSI(t *testing.T) {
	f, err := Parse("Events:")
	if err != nil {
		t.Fatal(err)
	}
	// Bold-color "Events:" prefix using a real ANSI sequence.
	colored := "\x1b[1mEvents:\x1b[0m payload"
	out := runStream(t, f, []string{"x", colored, "y"})
	if len(out) != 1 || !strings.Contains(out[0], "Events:") {
		t.Errorf("expected colored line emitted; got %v", out)
	}
	if !strings.Contains(out[0], "\x1b[1m") {
		t.Errorf("ANSI sequences should be preserved in emitted line; got %q", out[0])
	}
}

func TestApply_NilFilterIsIdentity(t *testing.T) {
	var f *Filter
	out := f.Apply("anything")
	if !reflect.DeepEqual(out, []string{"anything"}) {
		t.Errorf("nil Filter should pass through; got %v", out)
	}
}

func TestParse_Errors(t *testing.T) {
	cases := []string{
		"",
		"-A 2", // no pattern
		"hit -A",
		"hit -A abc",
		"hit extra-positional",
		"[invalid(regex",
	}
	for _, c := range cases {
		if _, err := Parse(c); err == nil {
			t.Errorf("expected error for %q, got nil", c)
		}
	}
}

func TestReset(t *testing.T) {
	f, err := Parse("hit -A 5")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Apply("hit") // arms after-context
	_ = f.Apply("a")
	f.Reset()
	// After reset, plain non-matching lines should not be emitted.
	out := f.Apply("b")
	if len(out) != 0 {
		t.Errorf("after Reset, expected no after-context emission; got %v", out)
	}
}

// Package grep implements a small grep-like line filter used to post-process
// km's colorized output. It exists so users can do
//
//	km describe pod foo --grep "Events: -A 50"
//
// and keep km's coloring — shelling out through a plain `grep` would strip
// the ANSI escapes.
//
// Supported flags inside the --grep argument (parsed like a mini grep command
// line): -A N / --after-context N, -B N / --before-context N, -C N /
// --context N, -i / --ignore-case, -v / --invert-match. The pattern is a Go
// regular expression. Pattern matching is done against the ANSI-stripped text
// so colors in the input don't affect matching, but the original (colored)
// line is what gets emitted.
package grep

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// ansiRegexp matches ANSI CSI escape sequences (color/format codes) so the
// pattern can match the visible text, not the ANSI codes.
var ansiRegexp = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// Filter is a streaming grep filter. Apply must be called in line order; it
// tracks before-context history and an after-context countdown across calls.
// A nil *Filter is the "no filter" identity (Apply returns the line as-is).
type Filter struct {
	pattern *regexp.Regexp
	after   int
	before  int
	invert  bool

	mu       sync.Mutex
	history  []string
	afterRem int
}

// Parse turns a grep-like argument string (e.g. "Events: -A 50 -i") into a
// Filter. The pattern is a Go regular expression. If a multi-word pattern is
// needed, escape spaces (`\s` or `[ ]`).
func Parse(arg string) (*Filter, error) {
	fields := strings.Fields(arg)
	if len(fields) == 0 {
		return nil, errors.New("--grep requires a pattern")
	}
	f := &Filter{}
	var pattern string
	var ignoreCase bool
	var contextSeen, afterSeen, beforeSeen bool
	var context int

	takeInt := func(flag string, i *int) error {
		if *i+1 >= len(fields) {
			return fmt.Errorf("--grep: %s requires a value", flag)
		}
		n, err := strconv.Atoi(fields[*i+1])
		if err != nil {
			return fmt.Errorf("--grep: %s: %v", flag, err)
		}
		*i++
		return setField(flag, n, &f.after, &f.before, &context, &afterSeen, &beforeSeen, &contextSeen)
	}

	for i := 0; i < len(fields); i++ {
		s := fields[i]
		switch {
		case s == "-A" || s == "--after-context":
			if err := takeInt(s, &i); err != nil {
				return nil, err
			}
		case strings.HasPrefix(s, "-A") && len(s) > 2:
			n, err := strconv.Atoi(s[2:])
			if err != nil {
				return nil, fmt.Errorf("--grep: -A: %v", err)
			}
			f.after = n
			afterSeen = true
		case s == "-B" || s == "--before-context":
			if err := takeInt(s, &i); err != nil {
				return nil, err
			}
		case strings.HasPrefix(s, "-B") && len(s) > 2:
			n, err := strconv.Atoi(s[2:])
			if err != nil {
				return nil, fmt.Errorf("--grep: -B: %v", err)
			}
			f.before = n
			beforeSeen = true
		case s == "-C" || s == "--context":
			if err := takeInt(s, &i); err != nil {
				return nil, err
			}
		case strings.HasPrefix(s, "-C") && len(s) > 2:
			n, err := strconv.Atoi(s[2:])
			if err != nil {
				return nil, fmt.Errorf("--grep: -C: %v", err)
			}
			context = n
			contextSeen = true
		case s == "-i" || s == "--ignore-case":
			ignoreCase = true
		case s == "-v" || s == "--invert-match":
			f.invert = true
		default:
			if pattern != "" {
				return nil, fmt.Errorf("--grep: unexpected argument %q (pattern already %q)", s, pattern)
			}
			pattern = s
		}
	}
	if pattern == "" {
		return nil, errors.New("--grep requires a pattern")
	}
	if contextSeen {
		if !afterSeen {
			f.after = context
		}
		if !beforeSeen {
			f.before = context
		}
	}
	expr := pattern
	if ignoreCase {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("--grep: invalid pattern %q: %v", pattern, err)
	}
	f.pattern = re
	return f, nil
}

// setField is a small helper used by Parse to record which numeric flag was
// supplied while still allowing -C to fall through when -A/-B aren't given.
func setField(flag string, n int, after, before, context *int, afterSeen, beforeSeen, contextSeen *bool) error {
	switch flag {
	case "-A", "--after-context":
		*after = n
		*afterSeen = true
	case "-B", "--before-context":
		*before = n
		*beforeSeen = true
	case "-C", "--context":
		*context = n
		*contextSeen = true
	}
	return nil
}

// Reset clears the before-context history and after-context counter. Call
// this between distinct streams (e.g. each watch redraw) so context from
// the previous stream doesn't leak into the next.
func (f *Filter) Reset() {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.history = f.history[:0]
	f.afterRem = 0
}

// Apply feeds one input line through the filter and returns the lines that
// should be emitted as a result. It may return zero, one, or many lines
// (many when before-context is flushed on a match).
func (f *Filter) Apply(line string) []string {
	if f == nil {
		return []string{line}
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	plain := ansiRegexp.ReplaceAllString(line, "")
	matched := f.pattern.MatchString(plain)
	if f.invert {
		matched = !matched
	}

	switch {
	case matched:
		out := make([]string, 0, len(f.history)+1)
		out = append(out, f.history...)
		f.history = f.history[:0]
		out = append(out, line)
		f.afterRem = f.after
		return out
	case f.afterRem > 0:
		f.afterRem--
		return []string{line}
	case f.before > 0:
		f.history = append(f.history, line)
		if len(f.history) > f.before {
			f.history = f.history[1:]
		}
	}
	return nil
}

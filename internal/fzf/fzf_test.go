package fzf_test

import (
	"errors"
	"testing"

	"github.com/catalinpan/km/internal/fzf"
	"github.com/ktr0731/go-fuzzyfinder"
)

type mockFinder struct {
	index int
	err   error
}

func (m *mockFinder) Find(items []string, _ func(int) string, _ ...fuzzyfinder.Option) (int, error) {
	if len(items) == 0 {
		return -1, errors.New("empty list")
	}
	return m.index, m.err
}

func TestSelect(t *testing.T) {
	t.Run("select from list", func(t *testing.T) {
		fzf.SetFinder(&mockFinder{index: 0})
		items := []string{"item1", "item2"}

		selected, err := fzf.Select(items, "test", func(s string) string { return "" })
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if selected != items[0] {
			t.Errorf("Expected %s, got %s", items[0], selected)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		fzf.SetFinder(&mockFinder{err: errors.New("empty list")})

		_, err := fzf.Select([]string{}, "test", nil)
		if err == nil {
			t.Error("Expected error for empty list")
		}
	})
}

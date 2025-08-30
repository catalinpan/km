package fzf

import (
	"sync"

	"github.com/ktr0731/go-fuzzyfinder"
)

type FuzzyFinderOption = fuzzyfinder.Option

// Finder interface for mocking
type Finder interface {
	Find(items []string, itemFunc func(int) string, opts ...FuzzyFinderOption) (int, error)
}

var finderImpl Finder = &defaultFinder{}

type defaultFinder struct{}

func (f *defaultFinder) Find(items []string, itemFunc func(int) string, opts ...fuzzyfinder.Option) (int, error) {
	return fuzzyfinder.Find(items, itemFunc, opts...)
}

// SetFinder sets the implementation for testing
func SetFinder(f Finder) {
	finderImpl = f
}

func GetFinder() Finder {
	return finderImpl
}

// Select shows a fuzzy selector for items with an optional preview.
// The preview is computed once per item (synchronously) and cached.
func Select(items []string, prompt string, previewFunc func(string) string) (string, error) {
	var (
		mu    sync.RWMutex
		cache = make(map[string]string)
	)

	// Single worker that always processes the most recent request
	// (coalesces bursts of selection changes).
	type req struct{ item string }
	reqCh := make(chan req, 1)

	// Panic-safe wrapper for user-provided previewFunc.
	runPreview := func(it string) (out string) {
		defer func() {
			if r := recover(); r != nil {
				out = "(preview error)"
			}
		}()
		return previewFunc(it)
	}

	// Start background worker only if we actually need previews.
	if previewFunc != nil {
		go func() {
			for r := range reqCh {
				// Drain to most recent request (drop stale work).
				for {
					select {
					case r = <-reqCh:
						continue
					default:
					}
					break
				}

				// Skip if already cached.
				mu.RLock()
				_, ok := cache[r.item]
				mu.RUnlock()
				if ok {
					continue
				}

				out := runPreview(r.item)

				mu.Lock()
				cache[r.item] = out
				mu.Unlock()
				// No explicit redraw API in go-fuzzyfinder; the preview callback
				// runs again on the next cursor change / keystroke and will pick up
				// the cached result. (Preview function is invoked when the selection
				// changes, per docs.) :contentReference[oaicite:0]{index=0}
			}
		}()
	}

	opts := []fuzzyfinder.Option{fuzzyfinder.WithPromptString(prompt)}
	if previewFunc != nil {
		var (
			mu    sync.RWMutex
			cache = make(map[string]string)
		)

		opts = append(opts, fuzzyfinder.WithPreviewWindow(func(i, _, _ int) string {
			if i < 0 {
				return ""
			}
			item := items[i]

			// Cached?
			mu.RLock()
			if s, ok := cache[item]; ok {
				mu.RUnlock()
				return s
			}
			mu.RUnlock()

			// Compute once (sync) then memoize; panic-safe
			var out string
			func() {
				defer func() {
					if r := recover(); r != nil {
						out = "(preview error)"
					}
				}()
				out = previewFunc(item)
			}()

			mu.Lock()
			cache[item] = out
			mu.Unlock()
			return out
		}))
	}

	idx, err := finderImpl.Find(
		items,
		func(i int) string { return items[i] },
		opts...,
	)

	// Close the worker after UI exits.
	if previewFunc != nil {
		close(reqCh)
	}

	if err != nil {
		return "", err
	}
	return items[idx], nil
}

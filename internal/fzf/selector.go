package fzf

import (
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

func Select(items []string, prompt string, previewFunc func(string) string) (string, error) {
	idx, err := finderImpl.Find(
		items,
		func(i int) string { return items[i] },
		fuzzyfinder.WithPromptString(prompt),
		fuzzyfinder.WithPreviewWindow(func(i, _, _ int) string {
			if i < 0 {
				return ""
			}
			return previewFunc(items[i])
		}),
	)
	if err != nil {
		return "", err
	}
	return items[idx], nil
}

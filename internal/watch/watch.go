// internal/watch/watch.go
package watch

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
)

// runFnWriter is the type of the function we’ll use inside watch.
// It must return the complete, colorized output buffer (as []byte).
// In practice, main.go will pass a wrapper around runKubectlToWriter.
type runFnWriter func([]string) ([]byte, error)

// HandleWatch is a “watch”-style loop that only rewrites changed lines.
//
//	rawArgs: everything after “watch” on the command line (e.g. ["-i","5","get","po"]).
//	runWriter: callback (usually a wrapper around runKubectlToWriter) that returns []byte of colorized kubectl output.
func HandleWatch(rawArgs []string, runWriter runFnWriter) {
	// default interval = 2s
	interval := 2
	var kubectlArgs []string

	if len(rawArgs) >= 2 && (rawArgs[0] == "-i" || rawArgs[0] == "--interval") {
		if n, err := strconv.Atoi(rawArgs[1]); err == nil && n > 0 {
			interval = n
			kubectlArgs = rawArgs[2:]
		} else {
			fmt.Fprintf(os.Stderr, "Invalid interval '%s'. Must be a positive integer.\n", rawArgs[1])
			os.Exit(1)
		}
	} else {
		kubectlArgs = rawArgs
	}

	if len(kubectlArgs) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: km watch [-i N] <kubectl-command> [args...]\n")
		os.Exit(1)
	}

	// If you want to use the alternate‐screen buffer you can still do it:
	fmt.Print("\033[?1049h") // enter alternate screen
	fmt.Print("\033[?25l")   // hide cursor
	cleanup := func() {
		fmt.Print("\033[?25h")   // restore cursor
		fmt.Print("\033[?1049l") // exit alternate screen
	}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cleanup()
		os.Exit(0)
	}()

	// Build the “km get …” string for the header
	cmdString := "km " + strings.Join(kubectlArgs, " ")

	// prevLines holds the last‐seen “body” (not including header)
	var prevLines []string

	// We’ll also track how many lines the header takes; for simplicity, assume it’s exactly 2 lines:
	//   1) header text
	//   2) blank line
	// (If you ever want multi‐line headers, you could adjust this count.)
	const headerHeight = 2

	// On the very first iteration, we want to print the header + all kubectl output in one shot.
	firstIteration := true

	for {
		// 1) Compute the current header
		host, _ := os.Hostname() // ignore errors; “unknown-host” is fine
		if host == "" {
			host = "unknown-host"
		}
		now := time.Now().Format("Mon Jan 02 15:04:05 MST 2006")

		leftText := fmt.Sprintf("Every %ds: %s", interval, cmdString)
		rightText := fmt.Sprintf("%s: %s", host, now)

		width, _, err := term.GetSize(int(os.Stdout.Fd()))
		if err != nil || width <= 0 {
			width = 80
		}
		leftLen := utf8.RuneCountInString(leftText)
		rightLen := utf8.RuneCountInString(rightText)
		padding := width - leftLen - rightLen
		if padding < 1 {
			padding = 1
		}
		headerLine := leftText + strings.Repeat(" ", padding) + rightText

		// 2) Run kubectl into a buffer:
		buf, err := runWriter(kubectlArgs)
		if err != nil {
			// If kubectl fails, restore terminal and exit
			cleanup()
			fmt.Fprintf(os.Stderr, "Error executing kubectl: %v\n", err)
			os.Exit(1)
		}

		// 3) Split buf into lines. We’ll drop any trailing empty line:
		allLines := strings.Split(string(buf), "\n")
		// If the last element is empty (because buf ended with “\n”), drop it
		if len(allLines) > 0 && allLines[len(allLines)-1] == "" {
			allLines = allLines[:len(allLines)-1]
		}

		if firstIteration {
			// On the first pass, print header + blank line + entire output:
			fmt.Print("\033[H\033[J") // move to top-left & clear downwards
			fmt.Println(headerLine)
			fmt.Println() // blank line
			for _, line := range allLines {
				fmt.Println(line)
			}
			prevLines = append(prevLines[:0], allLines...) // copy
			firstIteration = false

		} else {
			// We’re NOT on the first pass. Instead of a full clear, we only rewrite changed rows.

			// 3a) Print the header + blank line (always rewrite header because timestamp changes)
			// Move cursor to row 1, col 1:
			fmt.Print("\033[1;1H")
			fmt.Print("\033[K") // clear that entire line
			fmt.Println(headerLine)
			// Row 2 is just “blank,” so clear it:
			fmt.Print("\033[2;1H\033[K")
			fmt.Println()

			// 3b) Now diff prevLines vs allLines. We start printing body at row 3.
			// Let newLen = len(allLines), oldLen = len(prevLines).
			newLen := len(allLines)
			oldLen := len(prevLines)

			// For every i < min(newLen, oldLen), check if the line changed:
			minLen := newLen
			if oldLen < minLen {
				minLen = oldLen
			}
			for i := 0; i < minLen; i++ {
				if allLines[i] != prevLines[i] {
					// Move cursor to row = headerHeight + i + 1 (since row 3 is i=0),
					// clear that line, then print new text:
					row := headerHeight + i + 1
					fmt.Printf("\033[%d;1H\033[K", row)
					fmt.Println(allLines[i])
				}
			}

			// 3c) If new output has extra lines beyond oldLen, print those:
			if newLen > oldLen {
				for i := oldLen; i < newLen; i++ {
					row := headerHeight + i + 1
					fmt.Printf("\033[%d;1H\033[K", row)
					fmt.Println(allLines[i])
				}
			}

			// 3d) If old output was longer than new output, we need to clear the leftover lines:
			if oldLen > newLen {
				for i := newLen; i < oldLen; i++ {
					row := headerHeight + i + 1
					// Move to that row & clear it; do NOT print anything else
					fmt.Printf("\033[%d;1H\033[K", row)
				}
			}

			// 3e) Finally, update prevLines to reflect the new state:
			prevLines = append(prevLines[:0], allLines...)
		}

		// 4) Sleep, then repeat
		time.Sleep(time.Duration(interval) * time.Second)
	}
}

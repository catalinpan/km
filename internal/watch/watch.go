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

// HandleWatch is a “watch”-style loop that fully redraws the screen each tick.
// We disable line-wrap and avoid printing trailing newlines during drawing to
// prevent terminal scroll and duplicate/garbled rows when lines are wide.
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

	// Enter alternate screen, hide cursor, and DISABLE WRAP.
	fmt.Print("\033[?1049h") // alternate screen
	fmt.Print("\033[?25l")   // hide cursor
	fmt.Print("\033[?7l")    // disable line wrap

	cleanup := func() {
		fmt.Print("\033[?7h")    // re-enable line wrap
		fmt.Print("\033[?25h")   // show cursor
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

	// Header takes exactly 2 rows: (1) header text, (2) blank line
	const headerHeight = 2

	for {
		// 1) Compute the current header
		host, _ := os.Hostname()
		if host == "" {
			host = "unknown-host"
		}
		now := time.Now().Format("Mon Jan 02 15:04:05 MST 2006")

		leftText := fmt.Sprintf("Every %ds: %s", interval, cmdString)
		rightText := fmt.Sprintf("%s: %s", host, now)

		width, height, err := term.GetSize(int(os.Stdout.Fd()))
		if err != nil || width <= 0 {
			width = 80
		}
		if height <= 0 {
			height = 24
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
		if err != nil && len(buf) == 0 {
			// If kubectl fails with no output, show a generic error message and continue watching
			errMsg := fmt.Sprintf("Error executing kubectl: %v\n", err)
			buf = []byte(errMsg)
		}

		// 3) Split buf into lines. Drop trailing empty line if present.
		allLines := strings.Split(string(buf), "\n")
		if len(allLines) > 0 && allLines[len(allLines)-1] == "" {
			allLines = allLines[:len(allLines)-1]
		}

		// 4) Full redraw with absolute cursor addressing; no Println during draw.
		fmt.Print("\033[H\033[J")                   // home + clear
		fmt.Printf("\033[1;1H\033[K%s", headerLine) // header on row 1
		fmt.Print("\033[2;1H\033[K")                // blank row 2
		maxBody := height - headerHeight            // rows available for body
		if maxBody < 0 {
			maxBody = 0
		}

		if len(allLines) <= maxBody {
			// Everything fits; draw all body lines.
			for i := 0; i < len(allLines); i++ {
				row := headerHeight + i + 1 // body starts at row 3
				fmt.Printf("\033[%d;1H\033[K%s", row, allLines[i])
			}
		} else {
			// Truncate: reserve the last visible row for an ellipsis message.
			if maxBody >= 1 {
				show := maxBody - 1
				for i := 0; i < show; i++ {
					row := headerHeight + i + 1
					fmt.Printf("\033[%d;1H\033[K%s", row, allLines[i])
				}
				row := headerHeight + show + 1
				more := len(allLines) - show
				fmt.Printf("\033[%d;1H\033[K… (%d more lines)", row, more)
			}
			// If maxBody == 0: nothing to draw; header occupies the whole screen.
		}

		// 5) Sleep, then repeat
		time.Sleep(time.Duration(interval) * time.Second)
	}
}

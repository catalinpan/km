package completion_test

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/catalinpan/km/internal/completion"
)

func TestHandle(t *testing.T) {
	t.Run("bash completion", func(t *testing.T) {
		// Backup and restore original Exit
		oldExit := completion.Exit
		defer func() { completion.Exit = oldExit }()

		// Capture stdout
		r, w, _ := os.Pipe()
		oldStdout := os.Stdout
		os.Stdout = w
		defer func() { os.Stdout = oldStdout }()

		exitCode := -1
		completion.Exit = func(code int) { exitCode = code }

		completion.Handle([]string{"", "completion", "bash"})
		w.Close()

		var buf bytes.Buffer
		io.Copy(&buf, r)

		if exitCode != -1 {
			t.Errorf("Unexpected exit with code %d", exitCode)
		}
		if !bytes.Contains(buf.Bytes(), []byte("_km_completion")) {
			t.Error("Bash completion script not generated")
		}
	})

	t.Run("zsh completion", func(t *testing.T) {
		// Backup and restore original Exit
		oldExit := completion.Exit
		defer func() { completion.Exit = oldExit }()

		// Capture stdout
		r, w, _ := os.Pipe()
		oldStdout := os.Stdout
		os.Stdout = w
		defer func() { os.Stdout = oldStdout }()

		exitCode := -1
		completion.Exit = func(code int) { exitCode = code }

		completion.Handle([]string{"", "completion", "zsh"})
		w.Close()

		var buf bytes.Buffer
		io.Copy(&buf, r)

		if exitCode != -1 {
			t.Errorf("Unexpected exit with code %d", exitCode)
		}
		if !bytes.Contains(buf.Bytes(), []byte("#compdef km")) {
			t.Error("Zsh completion script not generated")
		}
		if !bytes.Contains(buf.Bytes(), []byte("_values 'namespaces'")) {
			t.Error("Missing namespace completion in zsh script")
		}
	})

	t.Run("invalid shell", func(t *testing.T) {
		// Backup and restore original Exit
		oldExit := completion.Exit
		defer func() { completion.Exit = oldExit }()

		// Capture stderr
		r, w, _ := os.Pipe()
		oldStderr := os.Stderr
		os.Stderr = w
		defer func() { os.Stderr = oldStderr }()

		exitCode := -1
		completion.Exit = func(code int) { exitCode = code }

		completion.Handle([]string{"", "completion", "invalid"})
		w.Close()

		var buf bytes.Buffer
		io.Copy(&buf, r)

		if exitCode != 1 {
			t.Error("Didn't exit with code 1")
		}
		if !bytes.Contains(buf.Bytes(), []byte("Unsupported shell type \"invalid\"")) {
			t.Error("Missing error message")
		}
	})
}

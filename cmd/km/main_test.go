package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"testing"
)

func TestMain(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantExit   int
		wantOutput string
		mockExec   func(string, ...string) *exec.Cmd
	}{
		{
			name:       "no args shows usage",
			args:       []string{},
			wantExit:   1,
			wantOutput: "km - Kubernetes Manager Wrapper",
			mockExec:   nil,
		},
		{
			name:       "help flag",
			args:       []string{"-h"},
			wantExit:   0,
			wantOutput: "Examples:",
			mockExec:   nil,
		},
		{
			name:       "invalid command",
			args:       []string{"invalid"},
			wantExit:   1,
			wantOutput: "Error executing kubectl: exit status 1",
			mockExec: func(_ string, _ ...string) *exec.Cmd {
				return exec.Command("false") // Returns exit status 1
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock exec if needed
			if tt.mockExec != nil {
				oldExec := execCommand
				execCommand = tt.mockExec
				defer func() { execCommand = oldExec }()
			}

			// Capture exit
			var exitCode int
			oldExit := osExit
			osExit = func(code int) { exitCode = code }
			defer func() { osExit = oldExit }()

			// Capture output
			oldStdout := os.Stdout
			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stdout = w
			os.Stderr = w
			defer func() {
				os.Stdout = oldStdout
				os.Stderr = oldStderr
			}()

			// Run test
			os.Args = append([]string{"km"}, tt.args...)
			main()
			w.Close()

			// Read output
			var buf bytes.Buffer
			io.Copy(&buf, r)

			// Verify exit code
			if exitCode != tt.wantExit {
				t.Errorf("Expected exit code %d, got %d", tt.wantExit, exitCode)
			}

			// Verify output
			if tt.wantOutput != "" && !bytes.Contains(buf.Bytes(), []byte(tt.wantOutput)) {
				t.Errorf("Missing %q in output: \n%s", tt.wantOutput, buf.String())
			}
		})
	}
}

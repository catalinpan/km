package testutil

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

var (
	origExecCommand = exec.Command
	mockExecCommand = exec.Command
	mu              sync.Mutex
)

func MockExecCommand(mockFn func(name string, arg ...string) *exec.Cmd) {
	mu.Lock()
	defer mu.Unlock()
	mockExecCommand = mockFn
}

func RestoreExecCommand() {
	mu.Lock()
	defer mu.Unlock()
	mockExecCommand = origExecCommand
}

func TestCmdSuccess(output string) *exec.Cmd {
	return TestCmd(output, nil)
}

func TestCmdError(output string, err error) *exec.Cmd {
	return TestCmd(output, err)
}

func TestCmd(output string, err error) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcess", "--"}
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{
		"GO_WANT_HELPER_PROCESS=1",
		fmt.Sprintf("GO_HELPER_OUTPUT=%s", output),
		fmt.Sprintf("GO_HELPER_ERROR=%v", err),
	}
	return cmd
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	args := os.Args[3:] // Actual command arguments
	if len(args) == 0 {
		os.Exit(0)
	}

	// Mock kubectl commands
	if args[0] == "kubectl" {
		switch args[1] {
		case "config":
			switch args[2] {
			case "get-contexts":
				fmt.Fprint(os.Stdout, "ctx1\nctx2\n")
			case "use-context":
				os.Exit(0) // Success for context switching
			case "view":
				handleConfigView(args[3:])
			}
		}
	}

	os.Exit(0)
}

func handleConfigView(args []string) {
	jsonpath := strings.Join(args[3:], " ") // Extract jsonpath
	switch {
	case strings.Contains(jsonpath, "context.cluster"):
		fmt.Fprint(os.Stdout, "mock-cluster")
	case strings.Contains(jsonpath, "context.user"):
		fmt.Fprint(os.Stdout, "mock-user")
	case strings.Contains(jsonpath, "context.namespace"):
		fmt.Fprint(os.Stdout, "mock-namespace")
	case strings.Contains(jsonpath, "cluster.server"):
		fmt.Fprint(os.Stdout, "https://mock-server:443")
	}
}

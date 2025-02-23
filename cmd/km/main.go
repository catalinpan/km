package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/catalinpan/km/internal/completion"
	"github.com/catalinpan/km/internal/kube"
)

var (
	execCommand = exec.Command
	osExit      = os.Exit
	// version is set at build time via ldflags. Default is "dev".
	version = "dev"
)

func main() {
	args := os.Args[1:]

	if hasHelpFlag(args) {
		printUsage()
		osExit(0)
		return
	}

	if hasVersionFlag(args) {
		fmt.Println("km version:", version)
		osExit(0)
		return
	}

	if len(args) == 0 {
		printUsage()
		osExit(1)
		return
	}

	switch args[0] {
	case "completion":
		completion.Handle(os.Args)
		osExit(0)
	case "cc":
		kube.HandleContextCommand()
		osExit(0)
	case "cn":
		handleCN(args)
	case "logs":
		kube.HandleLogsCommand(args[1:])
		osExit(0)
	default:
		if err := runKubectl(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error executing kubectl: %v\n", err)
			osExit(1)
			return // Prevent fallthrough to osExit(0)
		}
		osExit(0)
	}
}

func handleCN(args []string) {
	if len(args) > 1 {
		kube.ChangeNamespace(args[1])
	} else {
		ns, err := kube.GetValidNamespaceWithPodPreview()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error executing kubectl: %v\n", err)
			osExit(1)
		}
		kube.ChangeNamespace(ns)
	}
	osExit(0)
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

func hasVersionFlag(args []string) bool {
	for _, arg := range args {
		if arg == "-v" || arg == "--version" {
			return true
		}
	}
	return false
}

func printUsage() {
	fmt.Println(`km - Kubernetes Manager Wrapper
Usage:
  km <kubectl-command>     # Direct kubectl passthrough
  km cn [<namespace>]      # Change current namespace
  km logs [<pod>]          # View pod logs (interactive or direct)
  km cc                    # Switch cluster context
  km completion <shell>    # Generate completion script
                           # NOTE: kubectl completion must be installed locally
                           # echo 'source <(km completion bash)' >> ~/.bashrc
                           # echo 'source <(km completion zsh)' >> ~/.zshrc
Flags:
  -h, --help               Show km help
  -v, --version            Show km version
Examples:
  km get pods
  km cn monitoring
  km logs -f
  km cc`)
}

func runKubectl(args []string) error {
	cmd := execCommand("kubectl", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

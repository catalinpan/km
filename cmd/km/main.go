package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/catalinpan/km/internal/completion"
	"github.com/catalinpan/km/internal/kube"
	"github.com/fatih/color"
	"golang.org/x/term"
)

type colorizerState struct {
	statusIndex   int
	headerChecked bool
	isYAMLOutput  bool // Add this field
}

var (
	execCommand = exec.Command
	osExit      = os.Exit
	version     = "dev"

	// Color functions
	greenColor      = color.New(color.FgGreen).SprintFunc()
	redColor        = color.New(color.FgRed).SprintFunc()
	yellowColor     = color.New(color.FgYellow).SprintFunc()
	blueColor       = color.New(color.FgBlue).SprintFunc()
	magentaColor    = color.New(color.FgMagenta).SprintFunc()
	cyanColor       = color.New(color.FgCyan).SprintFunc()
	whiteColor      = color.New(color.FgHiWhite).SprintFunc()
	boldColor       = color.New(color.Bold).SprintFunc()
	yamlKeyColor    = color.New(color.FgRed).SprintFunc()
	yamlColonColor  = color.New(color.FgHiWhite).SprintFunc()
	yamlValueColor  = color.New(color.FgGreen).SprintFunc()
	yamlBoolColor   = color.New(color.FgMagenta).SprintFunc()
	yamlNumberColor = color.New(color.FgYellow).SprintFunc()
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
			return
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

func runRawKubectl(args []string) error {
	cmd := execCommand("kubectl", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
	// Check if command is get/describe
	if len(args) == 0 || (args[0] != "get" && args[0] != "describe") {
		return runRawKubectl(args)
	}
	// Detect YAML output format
	isYAML := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-oyaml" || arg == "-o=yaml" || arg == "--output=yaml":
			isYAML = true
		case arg == "-o" && i+1 < len(args) && args[i+1] == "yaml":
			isYAML = true
			i++ // Skip next argument
		}
	}
	cmd := execCommand("kubectl", args...)
	cmd.Stdin = os.Stdin

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	colorEnabledStdout := term.IsTerminal(int(os.Stdout.Fd()))
	colorEnabledStderr := term.IsTerminal(int(os.Stderr.Fd()))

	if err := cmd.Start(); err != nil {
		return err
	}

	// Process stdout
	go func() {
		state := &colorizerState{
			statusIndex:  2,
			isYAMLOutput: isYAML, // Set YAML flag
		}
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			if colorEnabledStdout {
				if state.isYAMLOutput {
					line = colorizeYAMLLine(line)
				} else {
					line = colorizeStdoutLine(line, state)
				}
			}
			fmt.Fprintln(os.Stdout, line)
		}
	}()

	// Process stderr
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			if colorEnabledStderr {
				line = redColor(line)
			}
			fmt.Fprintln(os.Stderr, line)
		}
	}()

	return cmd.Wait()
}

func colorizeStdoutLine(line string, state *colorizerState) string {
	// Handle table output
	if state != nil && (containsAnyHeader(line) || state.headerChecked) {
		return colorizeTableOutput(line, state)
	}
	// Handle describe output
	return colorizeDescriptionLine(line)
}

func colorizeTableOutput(line string, state *colorizerState) string {
	re := regexp.MustCompile(`(\s{2,})`)
	parts := re.Split(line, -1)
	spaces := re.FindAllString(line, -1)

	// Detect header and status column index
	if !state.headerChecked {
		for i, part := range parts {
			if strings.TrimSpace(part) == "STATUS" {
				state.statusIndex = i
				state.headerChecked = true
				break
			}
		}
		return boldColor(line) // Make header bold
	}

	// Color status column if exists
	if state.statusIndex < len(parts) {
		status := strings.TrimSpace(parts[state.statusIndex])
		switch status {
		case "Running", "Completed":
			parts[state.statusIndex] = greenColor(status)
		case "Error", "CrashLoopBackOff", "ErrImagePull", "Evicted", "Unknown", "CreateContainerConfigError", "OOMKilled", "ContainerCannotRun", "ContainerStatusUnknown":
			parts[state.statusIndex] = redColor(status)
		case "Pending", "ContainerCreating", "NotReady":
			parts[state.statusIndex] = yellowColor(status)
		}
	}

	// Rebuild line with original spacing
	var b strings.Builder
	for i := 0; i < len(parts); i++ {
		if i > 0 && i-1 < len(spaces) {
			b.WriteString(spaces[i-1])
		}
		b.WriteString(parts[i])
	}
	return b.String()
}

func containsAnyHeader(line string) bool {
	headers := []string{"NAME", "STATUS", "READY", "RESTARTS", "AGE", "NAMESPACE"}
	for _, header := range headers {
		if strings.Contains(line, header) {
			return true
		}
	}
	return false
}

func colorizeDescriptionLine(line string) string {
	// Handle Status line first (special case)
	if strings.HasPrefix(strings.TrimSpace(line), "Status:") {
		return colorizeStatusLine(line)
	}

	// Rule 1: Color cyan only for uppercase-starting keys with colon
	if matches := regexp.MustCompile(`^(\s*)([A-Z][^:]*:)`).FindStringSubmatch(line); len(matches) == 3 {
		// Skip annotation-like lines (containing periods or slashes before colon)
		if strings.ContainsAny(matches[2], "./") {
			return line
		}
		return cyanColor(matches[0]) + strings.TrimPrefix(line, matches[0])
	}

	// Rule 2: Color magenta for camelCase words without colon
	if matches := regexp.MustCompile(`^(\s*)([A-Z][a-zA-Z]+)(\s+)`).FindStringSubmatch(line); len(matches) == 4 {
		if matches[2] == "Type" {
			return line
		}
		return matches[1] + magentaColor(matches[2]) + matches[3] + strings.TrimPrefix(line, matches[0])
	}

	return line
}

func colorizeStatusLine(line string) string {
	// Existing status coloring logic (preserved)
	re := regexp.MustCompile(`^(\s*Status:\s+)(.*)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) == 3 {
		key := cyanColor("Status:")
		value := strings.TrimSpace(matches[2])

		switch value {
		case "Running", "Completed":
			value = greenColor(value)
		case "Pending", "ContainerCreating":
			value = yellowColor(value)
		case "Error", "Failed", "CrashLoopBackOff":
			value = redColor(value)
		}

		spacing := strings.Repeat(" ", len(matches[1])-len("Status:"))
		return key + spacing + value
	}
	return line
}

func colorizeYAMLLine(line string) string {
	if colonIndex := strings.Index(line, ":"); colonIndex > -1 {
		return yamlKeyColor(line[:colonIndex]) + line[colonIndex:]
	}
	return line
}

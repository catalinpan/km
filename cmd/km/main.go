package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/catalinpan/km/internal/completion"
	"github.com/catalinpan/km/internal/kube"
	"github.com/catalinpan/km/internal/watch"
	"github.com/fatih/color"
	"golang.org/x/term"
)

type colorizerState struct {
	statusIndex   int
	headerChecked bool
	isYAMLOutput  bool
	inTable       bool // NEW: track when we're inside a table block
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
	yamlBoolColor   = color.New(color.FgYellow).SprintFunc()
	yamlNumberColor = color.New(color.FgYellow).SprintFunc()
	// Regular expression to match numbers, quoted strings, and size values
	numberPattern       = regexp.MustCompile(`^-?\d+(\.\d+)?([eE][-+]?\d+)?$`)
	sizePattern         = regexp.MustCompile(`^\d+[a-zA-Z]+$`)
	quotedStringPattern = regexp.MustCompile(`^["']([^'\@#$%^&*()_{}\[\]|\\;:,<>/?!]*)["']$`)
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
	case "watch":
		watch.HandleWatch(args[1:], wrapperRunKubectlForWatch)
		osExit(0)

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
func wrapperRunKubectlForWatch(args []string) ([]byte, error) {
	return runKubectlToWriter(args, nil)
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
  km watch [-i N]          # “watch”-style loop with color
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
  km cc
  km watch get pods -o wide
  km watch -i 5 get pods -o wide`)
}

func runKubectl(args []string) error {
	// Check if command is get/describe
	if len(args) == 0 || (args[0] != "get" && args[0] != "describe" && args[0] != "top" && args[0] != "api-resources") {
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

	// Create channels to collect lines from stdout and stderr
	stdoutCh := make(chan string)
	stderrCh := make(chan string)

	// Read stdout lines and send to channel
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			stdoutCh <- scanner.Text()
		}
		close(stdoutCh)
	}()

	// Read stderr lines and send to channel
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			stderrCh <- scanner.Text()
		}
		close(stderrCh)
	}()

	state := &colorizerState{
		statusIndex:  -1,
		isYAMLOutput: isYAML,
		inTable:      false,
	}

	// Process lines from both channels sequentially
	stdoutDone, stderrDone := false, false
	for !stdoutDone || !stderrDone {
		select {
		case line, ok := <-stdoutCh:
			if !ok {
				stdoutDone = true
				continue
			}
			if colorEnabledStdout {
				if state.isYAMLOutput {
					line = colorizeYAMLLine(line)
				} else {
					line = colorizeStdoutLine(line, state)
				}
			}
			fmt.Fprintln(os.Stdout, line)
		case line, ok := <-stderrCh:
			if !ok {
				stderrDone = true
				continue
			}
			if colorEnabledStderr {
				line = redColor(line)
			}
			fmt.Fprintln(os.Stderr, line)
		}
	}

	return cmd.Wait()
}

// Detect if this line is a kubectl table header.
// We look for typical ALL-CAPS header tokens that don't appear in data rows.
func isTableHeader(line string) bool {
	headers := []string{
		"NAME", "STATUS", "READY", "RESTARTS", "AGE", "NAMESPACE",
		"ROLES", "VERSION", "TYPE", "CLUSTER-IP", "EXTERNAL-IP", "PORT(S)", "IP", "NODE",
	}
	upper := line // header is already uppercase words; data rows won't match these tokens
	found := false
	for _, h := range headers {
		if strings.Contains(upper, h) {
			found = true
			break
		}
	}
	// Require "NAME" specifically to be extra safe
	return found && strings.Contains(upper, "NAME")
}

func colorizeStdoutLine(line string, state *colorizerState) string {
	trim := strings.TrimSpace(line)

	// Transition rules for table blocks
	if isTableHeader(line) {
		state.inTable = true
		// Recompute STATUS index on every header line
		return colorizeTableOutput(line, state)
	}
	if state.inTable {
		if trim == "" {
			// Blank line ends a table block
			state.inTable = false
			state.headerChecked = false
			state.statusIndex = -1
			return line
		}
		return colorizeTableOutput(line, state)
	}

	// Not inside a table -> treat as describe/output text
	return colorizeDescriptionLine(line)
}

func colorizeTableOutput(line string, state *colorizerState) string {
	re := regexp.MustCompile(`(\s{2,})`)
	parts := re.Split(line, -1)
	spaces := re.FindAllString(line, -1)

	// If this is a header line, recompute the STATUS column index and bold the header.
	if isTableHeader(line) {
		state.statusIndex = -1
		for i, part := range parts {
			if strings.TrimSpace(part) == "STATUS" {
				state.statusIndex = i
				break
			}
		}
		state.headerChecked = true
		return boldColor(line)
	}

	// Data line: color the STATUS column if present
	if state.statusIndex >= 0 && state.statusIndex < len(parts) {
		status := strings.TrimSpace(parts[state.statusIndex])
		switch status {
		case "Running", "Completed", "Ready", "Active", "Bound":
			parts[state.statusIndex] = greenColor(status)
		case "Error", "CrashLoopBackOff", "ErrImagePull", "Evicted", "Unknown", "CreateContainerConfigError", "OOMKilled", "ContainerCannotRun", "NotReady,SchedulingDisabled", "ContainerStatusUnknown", "Failed":
			parts[state.statusIndex] = redColor(status)
		default:
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

func colorizeDescriptionLine(line string) string {
	// Handle Status line first (special case)
	if strings.HasPrefix(strings.TrimSpace(line), "Status:") {
		return colorizeStatusLine(line)
	}

	// Rule 1: Color cyan only for uppercase-starting keys with colon
	if matches := regexp.MustCompile(`^(\s*)([A-Z][^:\d]*:)`).FindStringSubmatch(line); len(matches) == 3 {
		// Skip annotation-like lines (containing periods or slashes before colon)
		if strings.ContainsAny(matches[2], "./") {
			return line
		}
		return cyanColor(matches[0]) + strings.TrimPrefix(line, matches[0])
	}

	// Rule 2: Color magenta
	if matches := regexp.MustCompile(`^(\s*)([a-zGM0-9-]+:)`).FindStringSubmatch(line); len(matches) == 3 {
		if matches[2] == "Type:" {
			return line
		}
		return matches[1] + magentaColor(matches[2]) + strings.TrimPrefix(line, matches[0])
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
	// Check for key: value lines first
	if colonIndex := strings.Index(line, ":"); colonIndex > -1 {
		key := line[:colonIndex]
		value := line[colonIndex+1:] // Preserve spaces around the colon
		// Trim spaces from value for boolean check
		trimmedValue := strings.TrimSpace(value)
		// Check if the trimmed value is a boolean
		if trimmedValue == "true" || trimmedValue == "false" || trimmedValue == "null" {
			return yamlKeyColor(key) + yamlColonColor(":") + yamlBoolColor(value)
		}
		if quotedStringPattern.MatchString(trimmedValue) {
			return yamlKeyColor(key) + yamlColonColor(":") + yamlNumberColor(value)
		}
		if numberPattern.MatchString(trimmedValue) {
			return yamlKeyColor(key) + yamlColonColor(":") + yamlNumberColor(value)
		}
		// Check if the trimmed value is a size value (e.g., 12Gi)
		if sizePattern.MatchString(trimmedValue) {
			return yamlKeyColor(key) + yamlColonColor(":") + yamlNumberColor(value)
		}
		// If not a special type, colorize value normally
		return yamlKeyColor(key) + yamlColonColor(":") + yamlValueColor(value)
	}

	// Handle list items (lines starting with '- ')
	if trimmed := strings.TrimLeft(line, " \t"); strings.HasPrefix(trimmed, "- ") {
		if parts := strings.SplitN(line, "- ", 2); len(parts) == 2 {
			// Preserve original indentation and dash, colorize the value
			return parts[0] + "- " + yamlValueColor(parts[1])
		}
	}

	// Return unmodified line if no patterns match
	return line
}

// runKubectlToWriter is identical to runKubectl, except that
// it writes all colored output into the given io.Writer (instead of os.Stdout).
// It returns the complete output as a []byte and any error.
func runKubectlToWriter(args []string, w io.Writer) ([]byte, error) {
	// “isYAML” detection and setting up cmd exactly like runKubectl ...
	isYAML := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-oyaml" || arg == "-o=yaml" || arg == "--output=yaml":
			isYAML = true
		case arg == "-o" && i+1 < len(args) && args[i+1] == "yaml":
			isYAML = true
			i++
		}
	}

	cmd := execCommand("kubectl", args...)
	cmd.Stdin = os.Stdin

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	colorEnabled := true

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// Read lines from both stdout and stderr, color them, and append into a single buffer.
	var buf bytes.Buffer
	stdoutCh := make(chan string)
	stderrCh := make(chan string)

	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			stdoutCh <- scanner.Text()
		}
		close(stdoutCh)
	}()
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			stderrCh <- scanner.Text()
		}
		close(stderrCh)
	}()

	state := &colorizerState{statusIndex: -1, isYAMLOutput: isYAML, inTable: false}
	stdoutDone, stderrDone := false, false

	for !stdoutDone || !stderrDone {
		select {
		case line, ok := <-stdoutCh:
			if !ok {
				stdoutDone = true
				continue
			}
			if colorEnabled {
				if state.isYAMLOutput {
					line = colorizeYAMLLine(line)
				} else {
					line = colorizeStdoutLine(line, state)
				}
			}
			buf.WriteString(line + "\n")
		case line, ok := <-stderrCh:
			if !ok {
				stderrDone = true
				continue
			}
			if colorEnabled {
				line = redColor(line)
			}
			buf.WriteString(line + "\n")
		}
	}

	if err := cmd.Wait(); err != nil {
		return buf.Bytes(), err
	}
	return buf.Bytes(), nil
}

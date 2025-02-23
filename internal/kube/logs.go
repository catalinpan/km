package kube

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/catalinpan/km/internal/fzf"
)

func HandleLogsCommand(args []string) {
	if len(args) > 0 {
		execKubectlLogs(args)
		return
	}

	pods, err := getPods()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting pods: %v\n", err)
		os.Exit(1)
	}

	if len(pods) == 1 {
		streamLogs(pods[0])
		return
	}

	selectedPod, err := fzf.Select(pods, "Select pod:", getPodPreview)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Pod selection failed: %v\n", err)
		os.Exit(1)
	}

	streamLogs(selectedPod)
}

func getPods() ([]string, error) {
	output, err := exec.Command("kubectl", "get", "pods", "-o", "name").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get pods: %v\n%s", err, output)
	}
	return strings.Split(strings.TrimSpace(
		strings.ReplaceAll(string(output), "pod/", "")), "\n"), nil
}

func getPodPreview(pod string) string {
	output, _ := exec.Command("kubectl", "describe", "pod", pod).CombinedOutput()
	return string(output)
}

func streamLogs(pod string) {
	cmd := exec.Command("kubectl", "logs", "-f", pod, "--all-containers")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error streaming logs: %v\n", err)
		os.Exit(1)
	}
}

func execKubectlLogs(args []string) {
	cmd := exec.Command("kubectl", append([]string{"logs"}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error executing logs command: %v\n", err)
		os.Exit(1)
	}
}

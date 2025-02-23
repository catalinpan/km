package kube

import (
	"fmt"
	"os/exec"
	"strings"
)

func GetPods() ([]string, error) {
	cmd := exec.Command("kubectl", "get", "pods", "-o", "name")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get pods: %v\n%s", err, string(output))
	}
	return parsePods(string(output)), nil
}

func GetPodsInNamespace(namespace string) ([]string, error) {
	cmd := exec.Command("kubectl", "get", "pods", "-n", namespace)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get pods: %v\n%s", err, string(output))
	}
	return strings.Split(strings.TrimSpace(string(output)), "\n"), nil
}

func parsePods(output string) []string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	pods := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		pods = append(pods, strings.TrimPrefix(line, "pod/"))
	}
	return pods
}

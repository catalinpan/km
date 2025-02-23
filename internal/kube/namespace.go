package kube

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/catalinpan/km/internal/fzf"
)

func ChangeNamespace(namespace string) {
	if exists, _ := namespaceExists(namespace); !exists {
		fmt.Printf("Namespace '%s' not found. Selecting new namespace:\n", namespace)
		ns, err := GetValidNamespaceWithPodPreview()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error selecting namespace: %v\n", err)
			os.Exit(1)
		}
		namespace = ns
	}

	if err := setNamespace(namespace); err != nil {
		fmt.Fprintf(os.Stderr, "Error setting namespace: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Namespace set to '%s'\n", namespace)
}

func GetValidNamespaceWithPodPreview() (string, error) {
	namespaces, err := getNamespaces()
	if err != nil {
		return "", err
	}

	return fzf.Select(namespaces, "Select namespace:", func(ns string) string {
		pods, _ := exec.Command("kubectl", "get", "pods", "-n", ns).CombinedOutput()
		if len(pods) == 0 {
			return "No pods in namespace"
		}
		return string(pods)
	})
}

func getNamespaces() ([]string, error) {
	output, err := exec.Command("kubectl", "get", "namespaces", "-o", "name").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get namespaces: %v\n%s", err, output)
	}
	return strings.Split(strings.TrimSpace(
		strings.ReplaceAll(string(output), "namespace/", "")), "\n"), nil
}

func namespaceExists(ns string) (bool, error) {
	err := exec.Command("kubectl", "get", "namespace", ns).Run()
	return err == nil, nil
}

func setNamespace(ns string) error {
	currentCtx, err := exec.Command("kubectl", "config", "current-context").Output()
	if err != nil {
		return fmt.Errorf("failed to get current context: %v", err)
	}
	return exec.Command("kubectl", "config", "set-context",
		strings.TrimSpace(string(currentCtx)), "--namespace", ns).Run()
}

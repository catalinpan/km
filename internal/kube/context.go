package kube

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/catalinpan/km/internal/fzf"
)

func HandleContextCommand() {
	contexts, err := getAllContexts()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting contexts: %v\n", err)
		os.Exit(1)
	}

	selected, err := fzf.Select(contexts, "Select context:", getContextPreview)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error selecting context: %v\n", err)
		os.Exit(1)
	}

	if err := switchContext(selected); err != nil {
		fmt.Fprintf(os.Stderr, "Error switching context: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Switched to context: %s\n", selected)
}

func getAllContexts() ([]string, error) {
	cmd := exec.Command("kubectl", "config", "get-contexts", "-o", "name")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get contexts: %v\n%s", err, string(output))
	}
	return strings.Split(strings.TrimSpace(string(output)), "\n"), nil
}

func getContextPreview(ctx string) string {
	details, err := getContextDetails(ctx)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return formatContextDetails(details)
}

func getContextDetails(ctx string) (map[string]string, error) {
	clusterCmd := exec.Command("kubectl", "config", "view", "-o",
		fmt.Sprintf("jsonpath={.contexts[?(@.name==\"%s\")].context.cluster}", ctx))
	clusterOutput, err := clusterCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("cluster lookup failed: %v\n%s", err, clusterOutput)
	}

	userCmd := exec.Command("kubectl", "config", "view", "-o",
		fmt.Sprintf("jsonpath={.contexts[?(@.name==\"%s\")].context.user}", ctx))
	userOutput, _ := userCmd.CombinedOutput()

	namespaceCmd := exec.Command("kubectl", "config", "view", "-o",
		fmt.Sprintf("jsonpath={.contexts[?(@.name==\"%s\")].context.namespace}", ctx))
	namespaceOutput, _ := namespaceCmd.CombinedOutput()

	serverCmd := exec.Command("kubectl", "config", "view", "-o",
		fmt.Sprintf("jsonpath={.clusters[?(@.name==\"%s\")].cluster.server}", strings.TrimSpace(string(clusterOutput))))
	serverOutput, _ := serverCmd.CombinedOutput()

	return map[string]string{
		"name":      ctx,
		"cluster":   strings.TrimSpace(string(clusterOutput)),
		"user":      strings.TrimSpace(string(userOutput)),
		"namespace": strings.TrimSpace(string(namespaceOutput)),
		"server":    strings.TrimSpace(string(serverOutput)),
	}, nil
}

func formatContextDetails(details map[string]string) string {
	ns := details["namespace"]
	if ns == "" {
		ns = "default"
	}
	return fmt.Sprintf(`Context Name:  %s
Cluster Name:  %s
Server:        %s
Auth User:     %s
Namespace:     %s`,
		details["name"], details["cluster"], details["server"], details["user"], ns)
}

func switchContext(ctx string) error {
	return exec.Command("kubectl", "config", "use-context", ctx).Run()
}

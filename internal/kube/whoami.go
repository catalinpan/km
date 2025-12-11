package kube

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"gopkg.in/yaml.v3"
)

type KubeConfig struct {
	CurrentContext string `yaml:"current-context"`
	Contexts       []struct {
		Name    string `yaml:"name"`
		Context struct {
			Cluster   string `yaml:"cluster"`
			User      string `yaml:"user"`
			Namespace string `yaml:"namespace"`
		} `yaml:"context"`
	} `yaml:"contexts"`
	Users []struct {
		Name string `yaml:"name"`
		User struct {
			ClientCertificateData string      `yaml:"client-certificate-data"`
			ClientCertificate     string      `yaml:"client-certificate"`
			Token                 string      `yaml:"token"`
			Exec                  *ExecConfig `yaml:"exec"`
		} `yaml:"user"`
	} `yaml:"users"`
}

type ExecConfig struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
	Env     []struct {
		Name  string `yaml:"name"`
		Value string `yaml:"value"`
	} `yaml:"env"`
	APIVersion string `yaml:"apiVersion"`
}

type ExecCredential struct {
	Status struct {
		Token               string    `json:"token"`
		ExpirationTimestamp time.Time `json:"expirationTimestamp"`
	} `json:"status"`
}

// HandleWhoamiCommand displays information about the current Kubernetes context
func HandleWhoamiCommand() {
	// Get current context
	currentContextCmd := exec.Command("kubectl", "config", "current-context")
	currentContextOutput, err := currentContextCmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current context: %v\n", err)
		os.Exit(1)
	}
	currentContext := strings.TrimSpace(string(currentContextOutput))

	// Get kubeconfig path
	kubeConfigPath := os.Getenv("KUBECONFIG")
	if kubeConfigPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting home directory: %v\n", err)
			os.Exit(1)
		}
		kubeConfigPath = filepath.Join(home, ".kube", "config")
	}

	// Read and parse kubeconfig
	configData, err := os.ReadFile(kubeConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading kubeconfig: %v\n", err)
		os.Exit(1)
	}

	var config KubeConfig
	if err := yaml.Unmarshal(configData, &config); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing kubeconfig: %v\n", err)
		os.Exit(1)
	}

	// Find the current context details
	var user, cluster, namespace string
	for _, ctx := range config.Contexts {
		if ctx.Name == currentContext {
			user = ctx.Context.User
			cluster = ctx.Context.Cluster
			namespace = ctx.Context.Namespace
			if namespace == "" {
				namespace = "default"
			}
			break
		}
	}

	if user == "" {
		fmt.Fprintf(os.Stderr, "Error: Could not find context '%s' in kubeconfig\n", currentContext)
		os.Exit(1)
	}

	// Find authentication expiration (certificate or token)
	var authExpiration string
	for _, u := range config.Users {
		if u.Name == user {
			// Try to get token expiration first (for exec-based auth like Pinniped)
			if u.User.Exec != nil {
				authExpiration = getExecTokenExpiration(u.User.Exec)
				if authExpiration == "" {
					authExpiration = color.CyanString("Unable to determine token expiration")
				}
				break
			} else if u.User.Token != "" {
				authExpiration = parseJWTExpiration(u.User.Token)
				if authExpiration == "" {
					authExpiration = color.CyanString("Token configured (unable to parse expiration)")
				}
				break
			}

			// Fall back to certificate-based authentication
			var certData []byte
			var err error

			// Check for inline certificate data first
			if u.User.ClientCertificateData != "" {
				certData, err = base64.StdEncoding.DecodeString(u.User.ClientCertificateData)
				if err != nil {
					authExpiration = color.YellowString("Unable to decode certificate data")
					break
				}
			} else if u.User.ClientCertificate != "" {
				// Read from file
				certPath := u.User.ClientCertificate
				// Expand ~ to home directory
				if strings.HasPrefix(certPath, "~/") {
					home, _ := os.UserHomeDir()
					certPath = filepath.Join(home, certPath[2:])
				}
				certData, err = os.ReadFile(certPath)
				if err != nil {
					authExpiration = color.YellowString("Certificate file not found")
					break
				}
			} else {
				authExpiration = color.CyanString("No client certificate configured")
				break
			}

			// Parse the certificate
			block, _ := pem.Decode(certData)
			if block == nil {
				authExpiration = color.YellowString("Unable to parse certificate")
				break
			}

			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				authExpiration = color.YellowString("Unable to parse certificate")
				break
			}

			// Check expiration
			authExpiration = formatExpiration(cert.NotAfter)
			break
		}
	}

	// Display the information
	bold := color.New(color.Bold)
	white := color.New(color.FgHiWhite)

	fmt.Println()
	bold.Print("User:       ")
	fmt.Println(white.Sprint(user))

	bold.Print("Namespace:  ")
	fmt.Println(color.YellowString(namespace))

	bold.Print("Context:    ")
	fmt.Println(white.Sprint(currentContext))

	bold.Print("Cluster:    ")
	fmt.Println(white.Sprint(cluster))

	if authExpiration != "" {
		bold.Print("Expires:    ")
		fmt.Println(authExpiration)
	}
	fmt.Println()
}

// getExecTokenExpiration executes the credential provider and gets token expiration
func getExecTokenExpiration(execConfig *ExecConfig) string {
	if execConfig == nil {
		return ""
	}

	// Execute the credential provider command
	cmd := exec.Command(execConfig.Command, execConfig.Args...)

	// Set environment variables if specified
	if len(execConfig.Env) > 0 {
		env := os.Environ()
		for _, e := range execConfig.Env {
			env = append(env, fmt.Sprintf("%s=%s", e.Name, e.Value))
		}
		cmd.Env = env
	}

	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Parse the ExecCredential response
	var cred ExecCredential
	if err := json.Unmarshal(output, &cred); err != nil {
		return ""
	}

	// Check if we have an expiration timestamp
	if !cred.Status.ExpirationTimestamp.IsZero() {
		return formatExpiration(cred.Status.ExpirationTimestamp)
	}

	// If no expiration timestamp, try to parse the token as JWT
	if cred.Status.Token != "" {
		return parseJWTExpiration(cred.Status.Token)
	}

	return ""
}

// parseJWTExpiration extracts expiration from a JWT token
func parseJWTExpiration(token string) string {
	// JWT format: header.payload.signature
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}

	// Decode the payload (second part)
	payload := parts[1]

	// Add padding if needed
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}

	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return ""
	}

	// Parse the JSON to get expiration
	var claims struct {
		Exp int64 `json:"exp"`
	}

	if err := json.Unmarshal(decoded, &claims); err != nil {
		return ""
	}

	if claims.Exp == 0 {
		return ""
	}

	expiryTime := time.Unix(claims.Exp, 0)
	return formatExpiration(expiryTime)
}

// formatExpiration formats an expiration time with color coding
func formatExpiration(expiryTime time.Time) string {
	now := time.Now()
	timeUntilExpiry := expiryTime.Sub(now)

	if now.After(expiryTime) {
		return color.RedString("EXPIRED on %s", expiryTime.Format("2006-01-02 15:04:05 MST"))
	} else if timeUntilExpiry < 24*time.Hour {
		return color.RedString("%s (expires in %s)", expiryTime.Format("2006-01-02 15:04:05 MST"), formatDuration(timeUntilExpiry))
	} else if timeUntilExpiry < 7*24*time.Hour {
		return color.YellowString("%s (expires in %s)", expiryTime.Format("2006-01-02 15:04:05 MST"), formatDuration(timeUntilExpiry))
	} else {
		return color.GreenString("%s (expires in %s)", expiryTime.Format("2006-01-02 15:04:05 MST"), formatDuration(timeUntilExpiry))
	}
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d hours", int(d.Hours()))
	}
	days := int(d.Hours() / 24)
	if days < 30 {
		return fmt.Sprintf("%d days", days)
	}
	months := days / 30
	if months < 12 {
		return fmt.Sprintf("%d months", months)
	}
	years := months / 12
	return fmt.Sprintf("%d years", years)
}

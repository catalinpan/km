# km

`km` is a thin wrapper for kubectl to enhance Kubernetes workflow with colorized output, context switching, and advanced log streaming.

Any command not recognized by `km` will be proxied directly to kubectl, so `km` can be used as a drop-in replacement for `kubectl`.

Check `km -h` for all available options.

## Demo
![Demo](examples/demo.gif)

## Features

- **Colorized kubectl output** - Automatically colorizes `get`, `describe`, and `top` commands
- **Context switching** - Quick cluster and namespace context changes with fuzzy finder
- **Session information** - Displays current user, namespace, cluster, and authentication expiration. Supports both certificate-based and exec-based authentication (Pinniped, OIDC, etc.).
- **Advanced log streaming** - Stream logs from multiple pods/containers simultaneously with colors (stern-like)
- **Watch mode** - Continuously refresh any kubectl command output
- **Shell completion** - Bash, Zsh, and Fish completion support
- **Single binary** - No dependencies required (kubectl must be installed separately)

## Install

### Linux
```bash
sudo curl -L \
$(curl -s https://api.github.com/repos/catalinpan/km/releases/latest \
| jq -r '.assets[] | select(.name == "km-linux-amd64") | .browser_download_url') \
-o /usr/local/bin/km && sudo chmod +x /usr/local/bin/km
```

### macOS
```bash
sudo curl -L \
$(curl -s https://api.github.com/repos/catalinpan/km/releases/latest \
| jq -r '.assets[] | select(.name == "km-darwin-amd64") | .browser_download_url') \
-o /usr/local/bin/km && sudo chmod +x /usr/local/bin/km
```

### From Source
```bash
git clone https://github.com/catalinpan/km.git
cd km
go build -o ./bin/km ./cmd/km/main.go
sudo mv ./bin/km /usr/local/bin/km
```

## Usage

### Change cluster context with fuzzy finder
```bash
km cc
```

### Change namespace context
```bash
# Interactive fuzzy finder with pod preview
km cn

# Direct namespace change
km cn default
km cn kube-system
```

### Show current session info
```bash
km whoami

User:       user-pinniped-example
Namespace:  myapp
Context:    user-pinniped-example@mycluster
Cluster:    mycluster
Expires:    2025-12-11 12:15:19 UTC (expires in 1 minutes)
```

Displays current user, namespace, cluster, and client certificate expiration.

### View pod logs

#### Interactive pod selection
```bash
km logs
```

#### Direct pod logs
```bash
km logs my-pod-name
km logs -f my-pod-name
```

#### Stream logs from all pods (stern-like)
```bash
# All pods in current namespace
km logs --all

# All pods in specific namespace
km logs --all -n kube-system

# Multiple namespaces
km logs --all -n velero -n external-dns -n monitoring

# With tail limit
km logs --all --tail 100
km logs --all -n production --tail 50

# Multiple namespaces with tail
km logs --all -n ns1 -n ns2 -n ns3 --tail 100
```

**Output format:**
```
namespace/pod-name › container-name | log message
```

Each pod gets a unique color (cycling through 6 colors), and container names are displayed in cyan for easy identification.

### Watch kubectl commands
```bash
# Default refresh (2 seconds)
km watch get pods -o wide

# Custom refresh interval
km watch -i 1 get pods -o wide
km watch -i 5 get deployments
```

### Shell completion

#### Bash
```bash
# Install completion
echo 'source <(km completion bash)' >> ~/.bashrc
source ~/.bashrc
```

#### Zsh
```bash
# Install completion
echo 'source <(km completion zsh)' >> ~/.zshrc
source ~/.zshrc
```

#### Fish
```bash
# Install completion
km completion fish | source
km completion fish > ~/.config/fish/completions/km.fish
```

**Note:** kubectl completion must be installed locally for km completion to work properly.

### Use as kubectl replacement

Any unrecognized command is passed directly to kubectl:
```bash
km get pods
km describe pod my-pod
km apply -f deployment.yaml
km delete pod my-pod
```

## Commands Reference

```
Usage:
  km <kubectl-command>     # Direct kubectl passthrough
  km cn [<namespace>]      # Change current namespace
  km logs [<pod>]          # View pod logs (interactive or direct)
  km logs --all [flags]    # Stream logs from all pods
  km cc                    # Switch cluster context
  km whoami                # Show current user, namespace, cluster, and auth expiration
  km watch [-i N]          # "watch"-style loop with color
  km completion <shell>    # Generate completion script

Logs --all flags:
  -n, --namespace NS       Namespace (can be specified multiple times)
  --tail N                 Number of lines to show from end of logs

Flags:
  -h, --help               Show km help
  -v, --version            Show km version
```

## Development

### Prerequisites
- Go 1.21 or higher
- kubectl installed and configured
- Access to a Kubernetes cluster

### Run from source
```bash
go run ./cmd/km/main.go get pods
go run ./cmd/km/main.go logs --all -n kube-system
```

### Run tests
```bash
# Run all tests
go test -v ./...

# Run specific package tests
go test -v ./internal/kube
go test -v ./internal/completion
```

### Build
```bash
# Build for current platform
go build -o ./bin/km ./cmd/km/main.go

# Build for Linux
GOOS=linux GOARCH=amd64 go build -o ./bin/km-linux-amd64 ./cmd/km/main.go

# Build for macOS
GOOS=darwin GOARCH=amd64 go build -o ./bin/km-darwin-amd64 ./cmd/km/main.go

# Build for macOS ARM (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o ./bin/km-darwin-arm64 ./cmd/km/main.go

# Build for Windows
GOOS=windows GOARCH=amd64 go build -o ./bin/km-windows-amd64.exe ./cmd/km/main.go
```

### Build with version
```bash
VERSION=$(git describe --tags --always --dirty)
go build -ldflags "-X main.version=${VERSION}" -o ./bin/km ./cmd/km/main.go
```

## Dependencies

- [fatih/color](https://github.com/fatih/color) - Terminal color output
- [ktr0731/go-fuzzyfinder](https://github.com/ktr0731/go-fuzzyfinder) - Interactive fuzzy finder
- [golang.org/x/term](https://pkg.go.dev/golang.org/x/term) - Terminal utilities

## Examples

### Colorized output
```bash
# Get pods with colorized status
km get pods

# Describe with colorized output
km describe pod my-pod

# YAML output with syntax highlighting
km get pod my-pod -o yaml
```

### Multi-namespace log streaming
```bash
# Monitor all pods across multiple namespaces
km logs --all -n production -n staging -n monitoring

# Last 50 lines from each pod in multiple namespaces
km logs --all -n app1 -n app2 --tail 50
```

### Watch with colors
```bash
# Watch deployments with status colors
km watch get deployments

# Watch pods with auto-refresh every second
km watch -i 1 get pods -o wide
```

## License

MIT License - see LICENSE file for details

## Acknowledgments

- Inspired by [stern](https://github.com/stern/stern) for multi-pod log streaming
- Built on top of [kubectl](https://kubernetes.io/docs/reference/kubectl/)
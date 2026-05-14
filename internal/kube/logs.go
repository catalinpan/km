package kube

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/catalinpan/km/internal/fzf"
	"github.com/catalinpan/km/internal/grep"
	"github.com/fatih/color"
)

// LogFilter applies the same --grep filter from km to streamed log lines.
// Set by main before HandleLogsCommand runs; nil = no filtering.
var LogFilter *grep.Filter

// reconcileInterval is how often streamAllLogs polls for the current pod set
// in each watched namespace so that new pods get attached and deleted pods
// get cleaned up.
const reconcileInterval = 5 * time.Second

var (
	greenColor   = color.New(color.FgGreen).SprintFunc()
	yellowColor  = color.New(color.FgYellow).SprintFunc()
	blueColor    = color.New(color.FgBlue).SprintFunc()
	magentaColor = color.New(color.FgMagenta).SprintFunc()
	cyanColor    = color.New(color.FgCyan).SprintFunc()
	whiteColor   = color.New(color.FgHiWhite).SprintFunc()
	redColor     = color.New(color.FgRed).SprintFunc()
)

type logLine struct {
	pod       string
	container string
	line      string
	isError   bool
}

func HandleLogsCommand(args []string) {
	hasAllFlag := false
	var namespaces []string
	tailLines := 0
	var filteredArgs []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--all" {
			hasAllFlag = true
		} else if arg == "-n" || arg == "--namespace" {
			if i+1 < len(args) {
				namespaces = append(namespaces, args[i+1])
				i++
			}
		} else if strings.HasPrefix(arg, "-n=") {
			namespaces = append(namespaces, strings.TrimPrefix(arg, "-n="))
		} else if strings.HasPrefix(arg, "--namespace=") {
			namespaces = append(namespaces, strings.TrimPrefix(arg, "--namespace="))
		} else if arg == "--tail" {
			if i+1 < len(args) {
				if val, err := strconv.Atoi(args[i+1]); err == nil {
					tailLines = val
					i++
				}
			}
		} else if strings.HasPrefix(arg, "--tail=") {
			if val, err := strconv.Atoi(strings.TrimPrefix(arg, "--tail=")); err == nil {
				tailLines = val
			}
		} else {
			filteredArgs = append(filteredArgs, arg)
		}
	}

	if hasAllFlag {
		streamAllLogs(namespaces, tailLines)
		return
	}

	if len(filteredArgs) > 0 {
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

type podRef struct {
	name      string
	namespace string
}

func (p podRef) key() string {
	return p.namespace + "/" + p.name
}

func (p podRef) display() string {
	if p.namespace != "" {
		return p.namespace + "/" + p.name
	}
	return p.name
}

// podStream tracks one streaming pod so reconcile can cancel it when the pod
// disappears, and so a goroutine that exits on its own (e.g. kubectl logs -f
// returned because the pod was deleted) can remove itself from the active map
// only if it has not been replaced.
type podStream struct {
	cancel context.CancelFunc
}

// logsWatcher streams logs for all pods matching a namespace set and keeps
// the streaming set in sync with the live pod set via periodic reconciliation.
// New pods are picked up, deleted pods are torn down.
type logsWatcher struct {
	namespaces []string
	tailLines  int
	colors     []func(...interface{}) string

	mu       sync.Mutex
	active   map[string]*podStream
	colorIdx int

	logCh chan logLine
}

func streamAllLogs(namespaces []string, tailLines int) {
	w := &logsWatcher{
		namespaces: namespaces,
		tailLines:  tailLines,
		colors: []func(...interface{}) string{
			greenColor, yellowColor, blueColor, magentaColor, cyanColor, whiteColor,
		},
		active: make(map[string]*podStream),
		logCh:  make(chan logLine, 1000),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	w.reconcile(ctx)

	w.mu.Lock()
	initial := len(w.active)
	w.mu.Unlock()
	if initial == 0 {
		fmt.Fprintln(os.Stderr, "No pods found yet, watching for new pods...")
	} else {
		fmt.Fprintf(os.Stderr, "Streaming logs from %d pod(s), watching for changes...\n", initial)
	}

	go w.reconcileLoop(ctx)

	for {
		select {
		case log := <-w.logCh:
			prefix := fmt.Sprintf("%s › %s", log.pod, log.container)
			if log.isError {
				fmt.Fprintf(os.Stderr, "%s | %s\n", prefix, log.line)
				continue
			}
			rendered := fmt.Sprintf("%s | %s", prefix, log.line)
			for _, l := range LogFilter.Apply(rendered) {
				fmt.Println(l)
			}
		case <-sigCh:
			fmt.Fprintln(os.Stderr, "\nShutting down...")
			cancel()
			return
		}
	}
}

func (w *logsWatcher) reconcileLoop(ctx context.Context) {
	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.reconcile(ctx)
		}
	}
}

// reconcile lists the current pods in every watched namespace, cancels streams
// for pods that have gone away, and starts streams for pods that are new.
func (w *logsWatcher) reconcile(ctx context.Context) {
	current := w.listCurrentPods()

	w.mu.Lock()
	defer w.mu.Unlock()

	for key, stream := range w.active {
		if _, ok := current[key]; !ok {
			stream.cancel()
			delete(w.active, key)
		}
	}

	for key, pod := range current {
		if _, ok := w.active[key]; ok {
			continue
		}
		podCtx, podCancel := context.WithCancel(ctx)
		stream := &podStream{cancel: podCancel}
		w.active[key] = stream
		podColor := w.colors[w.colorIdx%len(w.colors)]
		w.colorIdx++
		go func(p podRef, c func(...interface{}) string, s *podStream) {
			w.streamPod(podCtx, p, c)
			// Stream exited on its own (process died, no more containers, etc).
			// Remove the entry only if it has not already been replaced by a
			// later reconcile so the next reconcile can restart it.
			w.mu.Lock()
			if w.active[p.key()] == s {
				delete(w.active, p.key())
			}
			w.mu.Unlock()
		}(pod, podColor, stream)
	}
}

func (w *logsWatcher) listCurrentPods() map[string]podRef {
	current := make(map[string]podRef)
	if len(w.namespaces) == 0 {
		pods, err := getPods()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting pods: %v\n", err)
			return current
		}
		for _, name := range pods {
			if name == "" {
				continue
			}
			ref := podRef{name: name}
			current[ref.key()] = ref
		}
		return current
	}
	for _, ns := range w.namespaces {
		pods, err := getPodsInNamespace(ns)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting pods in namespace %s: %v\n", ns, err)
			continue
		}
		for _, name := range pods {
			if name == "" {
				continue
			}
			ref := podRef{name: name, namespace: ns}
			current[ref.key()] = ref
		}
	}
	return current
}

func (w *logsWatcher) streamPod(ctx context.Context, p podRef, podColor func(...interface{}) string) {
	args := []string{"get", "pod", p.name, "-o", "jsonpath={.spec.containers[*].name}"}
	if p.namespace != "" {
		args = append(args, "-n", p.namespace)
	}
	output, err := exec.CommandContext(ctx, "kubectl", args...).Output()
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		w.logCh <- logLine{
			pod:     podColor(p.display()),
			line:    fmt.Sprintf("Error getting containers: %v", err),
			isError: true,
		}
		return
	}

	containers := strings.Fields(string(output))
	if len(containers) == 0 {
		return
	}

	var wg sync.WaitGroup
	for _, container := range containers {
		wg.Add(1)
		go func(cont string) {
			defer wg.Done()
			w.streamContainer(ctx, p, cont, podColor)
		}(container)
	}
	wg.Wait()
}

func (w *logsWatcher) streamContainer(ctx context.Context, p podRef, container string, podColor func(...interface{}) string) {
	args := []string{"logs", "-f", p.name, "-c", container}
	if p.namespace != "" {
		args = append(args, "-n", p.namespace)
	}
	if w.tailLines > 0 {
		args = append(args, "--tail", strconv.Itoa(w.tailLines))
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		w.emit(p, container, podColor, fmt.Sprintf("Error creating pipe: %v", err), true)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		w.emit(p, container, podColor, fmt.Sprintf("Error creating stderr pipe: %v", err), true)
		return
	}
	if err := cmd.Start(); err != nil {
		w.emit(p, container, podColor, fmt.Sprintf("Error starting: %v", err), true)
		return
	}

	var pipeWg sync.WaitGroup
	pipeWg.Add(2)
	go func() {
		defer pipeWg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			w.emit(p, container, podColor, scanner.Text(), false)
		}
	}()
	go func() {
		defer pipeWg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			w.emit(p, container, podColor, scanner.Text(), true)
		}
	}()

	cmd.Wait()
	pipeWg.Wait()
}

func (w *logsWatcher) emit(p podRef, container string, podColor func(...interface{}) string, line string, isError bool) {
	w.logCh <- logLine{
		pod:       podColor(p.display()),
		container: cyanColor(container),
		line:      line,
		isError:   isError,
	}
}

func getPodsInNamespace(namespace string) ([]string, error) {
	args := []string{"get", "pods", "-n", namespace, "-o", "name"}
	output, err := exec.Command("kubectl", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get pods: %v\n%s", err, output)
	}
	return strings.Split(strings.TrimSpace(
		strings.ReplaceAll(string(output), "pod/", "")), "\n"), nil
}
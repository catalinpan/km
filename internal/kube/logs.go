package kube

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/catalinpan/km/internal/fzf"
	"github.com/fatih/color"
)

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

func streamAllLogs(namespaces []string, tailLines int) {
	type podRef struct {
		name      string
		namespace string
	}

	var allPods []podRef

	if len(namespaces) == 0 {
		pods, err := getPods()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting pods: %v\n", err)
			os.Exit(1)
		}
		for _, pod := range pods {
			allPods = append(allPods, podRef{name: pod, namespace: ""})
		}
	} else {
		for _, ns := range namespaces {
			pods, err := getPodsInNamespace(ns)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error getting pods in namespace %s: %v\n", ns, err)
				continue
			}
			for _, pod := range pods {
				allPods = append(allPods, podRef{name: pod, namespace: ns})
			}
		}
	}

	if len(allPods) == 0 {
		fmt.Fprintf(os.Stderr, "No pods found\n")
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Streaming logs from %d pod(s)...\n", len(allPods))

	logCh := make(chan logLine, 1000)

	colors := []func(...interface{}) string{
		greenColor,
		yellowColor,
		blueColor,
		magentaColor,
		cyanColor,
		whiteColor,
	}

	for i, pod := range allPods {
		podColor := colors[i%len(colors)]
		go streamPodLogs(pod.name, pod.namespace, podColor, logCh, tailLines)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		for {
			select {
			case log, ok := <-logCh:
				if !ok {
					return
				}
				prefix := fmt.Sprintf("%s › %s", log.pod, log.container)
				if log.isError {
					fmt.Fprintf(os.Stderr, "%s | %s\n", prefix, log.line)
				} else {
					fmt.Printf("%s | %s\n", prefix, log.line)
				}
			case <-sigCh:
				fmt.Fprintln(os.Stderr, "\nShutting down...")
				os.Exit(0)
			}
		}
	}()

	<-sigCh
	close(logCh)
}

func streamPodLogs(pod, namespace string, podColor func(...interface{}) string, logCh chan<- logLine, tailLines int) {
	args := []string{"get", "pod", pod, "-o", "jsonpath={.spec.containers[*].name}"}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	output, err := exec.Command("kubectl", args...).CombinedOutput()
	if err != nil {
		logCh <- logLine{
			pod:     pod,
			line:    fmt.Sprintf("Error getting containers: %v", err),
			isError: true,
		}
		return
	}

	containers := strings.Fields(string(output))
	if len(containers) == 0 {
		return
	}

	for _, container := range containers {
		go func(cont string) {
			args := []string{"logs", "-f", pod, "-c", cont}
			if namespace != "" {
				args = append(args, "-n", namespace)
			}
			if tailLines > 0 {
				args = append(args, "--tail", strconv.Itoa(tailLines))
			}

			cmd := exec.Command("kubectl", args...)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				logCh <- logLine{
					pod:       pod,
					container: cont,
					line:      fmt.Sprintf("Error creating pipe: %v", err),
					isError:   true,
				}
				return
			}

			stderr, err := cmd.StderrPipe()
			if err != nil {
				logCh <- logLine{
					pod:       pod,
					container: cont,
					line:      fmt.Sprintf("Error creating stderr pipe: %v", err),
					isError:   true,
				}
				return
			}

			if err := cmd.Start(); err != nil {
				logCh <- logLine{
					pod:       pod,
					container: cont,
					line:      fmt.Sprintf("Error starting: %v", err),
					isError:   true,
				}
				return
			}

			podName := pod
			if namespace != "" {
				podName = namespace + "/" + pod
			}

			go func() {
				scanner := bufio.NewScanner(stdout)
				for scanner.Scan() {
					logCh <- logLine{
						pod:       podColor(podName),
						container: cyanColor(cont),
						line:      scanner.Text(),
						isError:   false,
					}
				}
			}()

			go func() {
				scanner := bufio.NewScanner(stderr)
				for scanner.Scan() {
					logCh <- logLine{
						pod:       podColor(podName),
						container: cyanColor(cont),
						line:      scanner.Text(),
						isError:   true,
					}
				}
			}()

			cmd.Wait()
		}(container)
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
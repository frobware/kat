package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/frobware/kat"
	"github.com/frobware/kat/namespace"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// excludeFlags implements flag.Value to handle repeatable --exclude
// flags.
type excludeFlags []string

func (e *excludeFlags) String() string {
	return strings.Join(*e, ",")
}

func (e *excludeFlags) Set(value string) error {
	patterns := strings.Split(value, ",")

	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern != "" {
			*e = append(*e, pattern)
		}
	}

	return nil
}

// streamingHandler manages namespace-specific streaming
type streamingHandler struct {
	katInstance   *kat.Kat
	since         time.Duration
	activeStreams map[string]context.CancelFunc
	mu            sync.RWMutex
}

func newStreamingHandler(katInstance *kat.Kat, since time.Duration) *streamingHandler {
	return &streamingHandler{
		katInstance:   katInstance,
		since:         since,
		activeStreams: make(map[string]context.CancelFunc),
	}
}

func (h *streamingHandler) OnNamespaceAdded(namespace string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.activeStreams[namespace]; exists {
		return nil
	}

	log.Printf("Starting to watch namespace: %s", namespace)

	ctx, cancel := context.WithCancel(context.Background())
	h.activeStreams[namespace] = cancel

	go func() {
		defer func() {
			h.mu.Lock()
			delete(h.activeStreams, namespace)
			h.mu.Unlock()
		}()

		if err := h.katInstance.StartStreaming(ctx, []string{namespace}, h.since); err != nil {
			log.Printf("Error streaming namespace %s: %v", namespace, err)
		}
	}()

	return nil
}

func (h *streamingHandler) OnNamespaceDeleted(namespace string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if cancel, exists := h.activeStreams[namespace]; exists {
		log.Printf("Stopping watch for deleted namespace: %s", namespace)
		cancel()
		delete(h.activeStreams, namespace)
	}

	return nil
}

func (h *streamingHandler) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for namespace, cancel := range h.activeStreams {
		log.Printf("Stopping watch for namespace: %s", namespace)
		cancel()
	}

	h.activeStreams = make(map[string]context.CancelFunc)
}

func main() {
	qps := flag.Float64("qps", 500, "Kubernetes client QPS")
	burst := flag.Int("burst", 1000, "Kubernetes client burst")
	kubeconfig := flag.String("kubeconfig", "", "Path to kubeconfig")
	since := flag.Duration("since", time.Minute, "Show logs since duration (e.g., 5m)")
	silent := flag.Bool("silent", false, "Disable console output for log lines")
	teeDir := flag.String("tee", "", "Directory to write logs to (optional)")
	useTempDir := flag.Bool("d", false, "Automatically create a temporary directory for logs")
	allowExisting := flag.Bool("allow-existing", false, "Allow logging to an existing directory (default: false)")
	showVersion := flag.Bool("version", false, "Show version information")
	allNamespaces := flag.Bool("A", false, "Watch all namespaces")

	var excludePatterns excludeFlags
	flag.Var(&excludePatterns, "exclude", "Comma-separated namespace patterns to exclude (repeatable)")

	flag.Parse()

	if *showVersion {
		info := getVersionInfo()
		fmt.Printf("kat version %s\n", info.Version)
		fmt.Printf("Commit time: %s\n", info.CommitTime)
		fmt.Printf("Go version: %s\n", info.GoVersion)
		fmt.Printf("Platform: %s\n", info.Platform)
		return
	}

	if *useTempDir {
		if *teeDir != "" {
			log.Fatalf("Cannot use --tee and -d together. Choose one.")
		}

		tempDir := "/tmp/kat-" + time.Now().Format(time.RFC3339)
		if _, err := os.Stat(tempDir); err == nil {
			log.Fatalf("Temporary directory %s already exists. This is unexpected.", tempDir)
		} else if !os.IsNotExist(err) {
			log.Fatalf("Error checking temporary directory %s: %v", tempDir, err)
		}

		if err := os.MkdirAll(tempDir, 0o755); err != nil {
			log.Fatalf("Error creating temporary directory %s: %v", tempDir, err)
		}

		*teeDir = tempDir
		log.Printf("Log directory: %s", tempDir)
	} else if *teeDir != "" {
		if _, err := os.Stat(*teeDir); err == nil && !*allowExisting {
			log.Fatalf("Directory %s already exists. Use --allow-existing to continue.", *teeDir)
		} else if err != nil && !os.IsNotExist(err) {
			log.Fatalf("Error checking directory %s: %v", *teeDir, err)
		}
	}

	kubeconfigPath := *kubeconfig
	if kubeconfigPath == "" {
		kubeconfigPath = clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		log.Fatalf("Error loading kubeconfig: %v", err)
	}

	config.QPS = float32(*qps)
	config.Burst = *burst

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating Kubernetes client: %v", err)
	}

	var includePatternStrings []string
	args := flag.Args()

	if *allNamespaces {
		includePatternStrings = []string{}
	} else if len(args) == 0 {
		namespace, _, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			clientcmd.NewDefaultClientConfigLoadingRules(),
			&clientcmd.ConfigOverrides{},
		).Namespace()
		if err != nil {
			log.Fatalf("Error determining current namespace: %v", err)
		}
		includePatternStrings = []string{namespace}
	} else {
		includePatternStrings = args
	}

	includePatterns, err := namespace.ParsePatterns(includePatternStrings)
	if err != nil {
		log.Fatalf("Error parsing include patterns: %v", err)
	}

	parsedExcludePatterns, err := namespace.ParsePatterns(excludePatterns)
	if err != nil {
		log.Fatalf("Error parsing exclude patterns: %v", err)
	}

	outputCfg := &kat.OutputConfig{
		TeeDir: *teeDir,
		Silent: *silent,
	}

	k := kat.New(clientset, outputCfg, &kat.Callbacks{
		OnError: func(err error) {
			log.Printf("Error: %v", err)
		},
		OnFileClosed: func(filePath string) {
			log.Println("Closed log file", filePath)
		},
		OnFileCreated: func(filePath string) {
			log.Println("Created log file", filePath)
		},
		OnLogLine: func(namespace, podName, containerName, line string) {
			if !*silent {
				fmt.Printf("[%s/%s:%s] %s\n", namespace, podName, containerName, line)
			}
		},
		OnStreamStart: func(namespace, podName, containerName string) {
			log.Printf("Started streaming logs: %s/%s:%s", namespace, podName, containerName)
		},
		OnStreamStop: func(namespace, podName, containerName string) {
			log.Printf("Stopped streaming logs: %s/%s:%s", namespace, podName, containerName)
		},
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	needsDiscovery := *allNamespaces || len(parsedExcludePatterns) > 0
	if !needsDiscovery {
		for _, pattern := range includePatterns {
			if strings.ContainsAny(pattern.String(), "*?[]") {
				needsDiscovery = true
				break
			}
		}
	}

	if needsDiscovery {
		handler := newStreamingHandler(k, *since)
		watcher := namespace.NewInformerWatcher(clientset)

		go func() {
			<-ctx.Done()
			log.Println("Shutting down...")
			handler.Stop()
			watcher.Stop()
			if err := k.StopStreaming(); err != nil {
				log.Printf("Error stopping streaming: %v", err)
			}
		}()

		if err := watcher.Start(ctx, includePatterns, parsedExcludePatterns, handler); err != nil {
			log.Fatalf("Error starting namespace watcher: %v", err)
		}

		<-ctx.Done()
		log.Println("Shutdown complete")
	} else {
		var namespaceNames []string
		for _, pattern := range includePatterns {
			namespaceNames = append(namespaceNames, pattern.String())
		}

		go func() {
			<-ctx.Done()
			log.Println("Shutting down...")
			if err := k.StopStreaming(); err != nil {
				log.Printf("Error stopping streaming: %v", err)
			}
		}()

		if err := k.StartStreaming(ctx, namespaceNames, *since); err != nil {
			log.Fatalf("Error starting streaming: %v", err)
		}

		log.Println("Shutdown complete")
	}
}

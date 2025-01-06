package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/frobware/kat"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	qps := flag.Float64("qps", 500, "Kubernetes client QPS")
	burst := flag.Int("burst", 1000, "Kubernetes client burst")
	kubeconfig := flag.String("kubeconfig", "", "Path to kubeconfig")
	since := flag.Duration("since", time.Minute, "Show logs since duration (e.g., 5m)")
	silent := flag.Bool("silent", false, "Disable console output for log lines")
	teeDir := flag.String("tee", "", "Directory to write logs to (optional)")
	useTempDir := flag.Bool("d", false, "Automatically create a temporary directory for logs")
	allowExisting := flag.Bool("allow-existing", false, "Allow logging to an existing directory (default: false)")

	flag.Parse()

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

	namespaces := flag.Args()
	if len(namespaces) == 0 {
		namespace, _, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			clientcmd.NewDefaultClientConfigLoadingRules(),
			&clientcmd.ConfigOverrides{},
		).Namespace()
		if err != nil {
			log.Fatalf("Error determining current namespace: %v", err)
		}

		namespaces = []string{namespace}
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
				log.Printf("[%s/%s:%s] %s", namespace, podName, containerName, line)
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

	go func() {
		<-ctx.Done()

		if err := k.StopStreaming(); err != nil {
			log.Printf("Error stopping streaming: %v", err)
		}
	}()

	if err := k.StartStreaming(ctx, namespaces, *since); err != nil {
		log.Printf("Error during streaming: %v", err)
	}
}

// kat (Kubernetes Attach & Tail) follows and streams logs from every
// container in every pod across specified namespaces in real-time. It
// automatically attaches to new pods as they start up and detaches
// when they terminate. Think of it as cat(1) and tail(1) combined,
// but for watching all container logs in your selected Kubernetes
// namespaces simultaneously.

package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

var activeStreams sync.Map

// outputConfig encapsulates configuration for controlling log output.
type outputConfig struct {
	teeDir string // Directory to write logs (optional).
	silent bool   // Suppress console log output.
}

func streamPodLogs(ctx context.Context, clientset *kubernetes.Clientset, namespace, podName string, since time.Duration, outputCfg *outputConfig) error {
	pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error getting pod %s: %w", podName, err)
	}

	var wg sync.WaitGroup
	for _, container := range pod.Spec.Containers {
		wg.Add(1)
		go func(containerName string) {
			defer wg.Done()
			log.Printf("Streaming logs for pod: %s/%s:%s\n", namespace, podName, containerName)

			req := clientset.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
				Container: containerName,
				Follow:    true,
				SinceTime: &metav1.Time{Time: time.Now().Add(-since)},
			})

			stream, err := req.Stream(context.Background())
			if err != nil {
				log.Printf("Error streaming logs for pod %s, container %s: %v\n", podName, containerName, err)
				return
			}
			defer stream.Close()

			var file *os.File
			if outputCfg.teeDir != "" {
				filePath := filepath.Join(outputCfg.teeDir, namespace, podName, fmt.Sprintf("%s.log", containerName))
				if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
					log.Printf("Error creating directories for %s: %v\n", filePath, err)
					return
				}
				file, err = os.Create(filePath)
				if err != nil {
					log.Printf("Error creating file %s: %v\n", filePath, err)
					return
				}
				log.Printf("Created log file: %s\n", filePath)
				defer func() {
					file.Close()
					log.Printf("Closed log file: %s\n", filePath)
				}()
			}

			scanner := bufio.NewScanner(stream)
			for scanner.Scan() {
				logLine := fmt.Sprintf("[%s/%s:%s] %s\n", namespace, podName, containerName, scanner.Text())
				if !outputCfg.silent {
					log.Print(logLine)
				}
				if file != nil {
					file.WriteString(scanner.Text() + "\n")
				}
			}
		}(container.Name)
	}
	wg.Wait()

	return nil
}

func startLogStream(ctx context.Context, clientset *kubernetes.Clientset, namespace, podName string, since time.Duration, outputCfg *outputConfig) {
	if _, exists := activeStreams.Load(podName); exists {
		return
	}

	podCtx, cancel := context.WithCancel(ctx)
	activeStreams.Store(podName, cancel)

	go func() {
		defer func() {
			cancel()
			activeStreams.Delete(podName)
		}()

		backoff := wait.Backoff{
			Steps:    5,
			Duration: 100 * time.Millisecond,
			Factor:   2.0,
			Jitter:   0.1,
		}

		err := wait.ExponentialBackoff(backoff, func() (bool, error) {
			if err := streamPodLogs(podCtx, clientset, namespace, podName, since, outputCfg); err != nil {
				return false, nil
			}
			return true, nil
		})
		if err != nil {
			log.Printf("Failed to start log streaming for pod %s after retries: %v\n", podName, err)
		}
	}()
}

func watchPods(ctx context.Context, clientset *kubernetes.Clientset, namespace string, since time.Duration, outputCfg *outputConfig) {
	podList, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("Error listing pods in namespace %s: %v\n", namespace, err)
		return
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			startLogStream(ctx, clientset, namespace, pod.Name, since, outputCfg)
		}
	}

	factory := informers.NewSharedInformerFactoryWithOptions(clientset, 0, informers.WithNamespace(namespace))
	podInformer := factory.Core().V1().Pods().Informer()

	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod := obj.(*corev1.Pod)
			if pod.Status.Phase == corev1.PodRunning {
				startLogStream(ctx, clientset, namespace, pod.Name, since, outputCfg)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldPod := oldObj.(*corev1.Pod)
			newPod := newObj.(*corev1.Pod)

			if newPod.Status.Phase == corev1.PodRunning && oldPod.Status.Phase != corev1.PodRunning {
				startLogStream(ctx, clientset, namespace, newPod.Name, since, outputCfg)
			} else if newPod.Status.Phase != corev1.PodRunning {
				stopLogStream(newPod.Name)
			}
		},
		DeleteFunc: func(obj interface{}) {
			pod := obj.(*corev1.Pod)
			stopLogStream(pod.Name)
		},
	})

	go podInformer.Run(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), podInformer.HasSynced) {
		log.Println("Failed to sync informer cache")
		return
	}

	<-ctx.Done()
}

func stopLogStream(podName string) {
	if cancel, ok := activeStreams.Load(podName); ok {
		cancel.(context.CancelFunc)()
		activeStreams.Delete(podName)
		log.Printf("Stopped log stream for pod: %s\n", podName)
	}
}

func main() {
	flag.Usage = func() {
		progname := filepath.Base(os.Args[0])
		fmt.Fprintf(os.Stderr, "%s: Stream Kubernetes pod logs across namespaces\n\n", progname)
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  %s [flags] [<namespace>...]\n\n", progname)
		fmt.Fprintf(os.Stderr, "Flags:\n")

		flag.PrintDefaults()
	}

	qpsPtr := flag.Float64("qps", 500, "Kubernetes client QPS")
	burstPtr := flag.Int("burst", 1000, "Kubernetes client burst")
	kubeconfigFlag := flag.String("kubeconfig", "", "Path to the kubeconfig file (defaults to ~/.kube/config)")
	sincePtr := flag.Duration("since", time.Minute, "Show logs since duration (e.g., 5m)")
	teeDir := flag.String("tee", "", "Directory to write logs to (optional)")
	silent := flag.Bool("silent", false, "Disable console output for log lines")

	flag.Parse()

	kubeconfigPath := *kubeconfigFlag
	if kubeconfigPath == "" {
		kubeconfigPath = clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
	}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	currentNamespace, _, err := clientConfig.Namespace()
	if err != nil {
		log.Printf("Error determining current namespace: %v\n", err)
		return
	}

	namespaces := flag.Args()
	if len(namespaces) == 0 {
		log.Printf("No namespaces specified; defaulting to the current namespace: %s\n", currentNamespace)
		namespaces = []string{currentNamespace}
	}

	log.Printf("Using kubeconfig: %s\n", kubeconfigPath)

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		log.Printf("Error loading kubeconfig: %v\n", err)
		return
	}

	config.QPS = float32(*qpsPtr)
	config.Burst = *burstPtr
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("Error creating Kubernetes client: %v\n", err)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	outputCfg := &outputConfig{
		teeDir: *teeDir,
		silent: *silent,
	}

	var wg sync.WaitGroup
	for _, namespace := range namespaces {
		wg.Add(1)
		go func(namespace string) {
			defer wg.Done()
			watchPods(ctx, clientset, strings.TrimSpace(namespace), *sincePtr, outputCfg)
		}(namespace)
	}
	wg.Wait()
}

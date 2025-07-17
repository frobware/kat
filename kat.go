// kat (Kubernetes Attach & Tail) follows and streams logs from every
// container in every pod across specified namespaces in real-time. It
// automatically attaches to new pods as they start up and detaches
// when they terminate. Think of it as cat(1) and tail(1) combined,
// but for watching all container logs in your selected Kubernetes
// namespaces simultaneously.
package kat

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// Callbacks provides hooks for progress updates.
type Callbacks struct {
	OnError       func(err error)
	OnFileClosed  func(filePath string)
	OnFileCreated func(filePath string)
	OnLogLine     func(namespace, podName, containerName, line string)
	OnStreamStart func(namespace, podName, containerName string)
	OnStreamStop  func(namespace, podName, containerName string)
}

// Kat represents the main structure for managing POD log streaming.
type Kat struct {
	clientset     *kubernetes.Clientset
	outputConfig  *OutputConfig
	activeStreams sync.Map
	openFiles     sync.Map
	callbacks     *Callbacks
}

// OutputConfig encapsulates configuration for controlling log output.
type OutputConfig struct {
	TeeDir string // Directory to write logs (optional).
	Silent bool   // Suppress console log output.
}

// New creates a new Kat instance.
func New(clientset *kubernetes.Clientset, outputConfig *OutputConfig, callbacks *Callbacks) *Kat {
	return &Kat{
		clientset:    clientset,
		outputConfig: outputConfig,
		callbacks:    callbacks,
	}
}

// StartStreaming begins streaming logs for the specified namespaces.
func (k *Kat) StartStreaming(ctx context.Context, namespaces []string, since time.Duration) error {
	var wg sync.WaitGroup

	errCh := make(chan error, len(namespaces))

	for _, namespace := range namespaces {
		wg.Add(1)

		go func(namespace string) {
			defer wg.Done()

			if err := k.watchPods(ctx, namespace, since); err != nil {
				errCh <- fmt.Errorf("namespace %s: %w", namespace, err)
			}
		}(namespace)
	}

	wg.Wait()
	close(errCh)

	var errs []error

	for err := range errCh {
		if k.callbacks != nil && k.callbacks.OnError != nil {
			k.callbacks.OnError(err)
		}

		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("streaming errors: %v", errs)
	}

	return nil
}

// StopStreaming stops all active log streams and closes open files.
func (k *Kat) StopStreaming() error {
	var errs []error

	k.activeStreams.Range(func(key, value any) bool {
		if cancel, ok := value.(context.CancelFunc); ok {
			cancel()
		}

		k.activeStreams.Delete(key)

		return true
	})

	k.openFiles.Range(func(key, value any) bool {
		if file, ok := value.(*os.File); ok {
			if err := file.Sync(); err != nil {
				errs = append(errs, fmt.Errorf("sync file %v: %w", key, err))
			}

			if err := file.Close(); err != nil {
				errs = append(errs, fmt.Errorf("close file %v: %w", key, err))
			}

			if k.callbacks != nil && k.callbacks.OnFileClosed != nil {
				k.callbacks.OnFileClosed(key.(string))
			}
		}

		k.openFiles.Delete(key)

		return true
	})

	if len(errs) > 0 {
		return fmt.Errorf("errors during cleanup: %v", errs)
	}

	return nil
}

func (k *Kat) watchPods(ctx context.Context, namespace string, since time.Duration) error {
	podList, err := k.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("error listing pods in namespace %s: %w", namespace, err)
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			k.startLogStream(ctx, namespace, pod.Name, since)
		}
	}

	factory := informers.NewSharedInformerFactoryWithOptions(k.clientset, 0, informers.WithNamespace(namespace))
	podInformer := factory.Core().V1().Pods().Informer()

	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			pod := obj.(*corev1.Pod)
			if pod.Status.Phase == corev1.PodRunning {
				k.startLogStream(ctx, namespace, pod.Name, since)
			}
		},
		UpdateFunc: func(oldObj, newObj any) {
			oldPod := oldObj.(*corev1.Pod)
			newPod := newObj.(*corev1.Pod)

			if newPod.Status.Phase == corev1.PodRunning && oldPod.Status.Phase != corev1.PodRunning {
				k.startLogStream(ctx, namespace, newPod.Name, since)
			} else if newPod.Status.Phase != corev1.PodRunning {
				k.stopLogStream(newPod.Name)
			}
		},
		DeleteFunc: func(obj any) {
			pod := obj.(*corev1.Pod)
			k.stopLogStream(pod.Name)
		},
	})

	go podInformer.Run(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), podInformer.HasSynced) {
		return fmt.Errorf("failed to sync informer cache for namespace %s", namespace)
	}

	<-ctx.Done()

	return nil
}

func (k *Kat) startLogStream(ctx context.Context, namespace, podName string, since time.Duration) {
	if _, exists := k.activeStreams.Load(podName); exists {
		return
	}

	podCtx, cancel := context.WithCancel(ctx)
	k.activeStreams.Store(podName, cancel)

	go func() {
		defer func() {
			cancel()
			k.activeStreams.Delete(podName)
		}()

		backoff := wait.Backoff{
			Steps:    5,
			Duration: 100 * time.Millisecond,
			Factor:   2.0,
			Jitter:   0.1,
		}

		_ = wait.ExponentialBackoff(backoff, func() (bool, error) {
			if err := k.streamPodLogs(podCtx, namespace, podName, since); err != nil {
				return false, err
			}

			return true, nil
		})
	}()
}

func (k *Kat) streamPodLogs(ctx context.Context, namespace, podName string, since time.Duration) error {
	pod, err := k.clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error getting pod %s: %w", podName, err)
	}

	var wg sync.WaitGroup
	for _, container := range pod.Spec.Containers {
		wg.Add(1)

		go func(containerName string) {
			defer wg.Done()

			if k.callbacks != nil && k.callbacks.OnStreamStart != nil {
				k.callbacks.OnStreamStart(namespace, podName, containerName)
			}

			req := k.clientset.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
				Container: containerName,
				Follow:    true,
				SinceTime: &metav1.Time{Time: time.Now().Add(-since)},
			})

			stream, err := req.Stream(ctx)
			if err != nil {
				if k.callbacks != nil && k.callbacks.OnError != nil {
					k.callbacks.OnError(fmt.Errorf("error streaming logs for pod %s, container %s: %w", podName, containerName, err))
				}

				return
			}
			defer stream.Close()

			var (
				file     *os.File
				filePath string
			)

			scanner := bufio.NewScanner(stream)
			for scanner.Scan() {
				line := scanner.Text()

				if file == nil && k.outputConfig.TeeDir != "" {
					filePath = filepath.Join(k.outputConfig.TeeDir, namespace, podName, containerName+".txt")
					if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
						if k.callbacks != nil && k.callbacks.OnError != nil {
							k.callbacks.OnError(fmt.Errorf("error creating directories for %s: %w", filePath, err))
						}

						return
					}

					file, err = os.Create(filePath)
					if err != nil {
						if k.callbacks != nil && k.callbacks.OnError != nil {
							k.callbacks.OnError(fmt.Errorf("error creating file %s: %w", filePath, err))
						}

						return
					}

					k.openFiles.Store(filePath, file)

					if k.callbacks != nil && k.callbacks.OnFileCreated != nil {
						k.callbacks.OnFileCreated(filePath)
					}
				}

				if k.callbacks != nil && k.callbacks.OnLogLine != nil {
					k.callbacks.OnLogLine(namespace, podName, containerName, line)
				}

				if file != nil {
					// TODO: handle write failures.
					file.WriteString(line + "\n")
				}
			}

			if file != nil {
				k.openFiles.Delete(filePath)
				file.Close()

				if k.callbacks != nil && k.callbacks.OnFileClosed != nil {
					k.callbacks.OnFileClosed(filePath)
				}
			}

			if k.callbacks != nil && k.callbacks.OnStreamStop != nil {
				k.callbacks.OnStreamStop(namespace, podName, containerName)
			}
		}(container.Name)
	}

	wg.Wait()

	return nil
}

func (k *Kat) stopLogStream(podName string) {
	if cancel, ok := k.activeStreams.Load(podName); ok {
		cancel.(context.CancelFunc)()
		k.activeStreams.Delete(podName)
	}
}

package namespace

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

type Pattern struct {
	original string
	isGlob   bool
}

func newPattern(pattern string) (*Pattern, error) {
	if pattern == "" {
		return nil, fmt.Errorf("pattern cannot be empty")
	}

	isGlob := strings.ContainsAny(pattern, "*?[]")

	if isGlob {
		if _, err := filepath.Match(pattern, "test"); err != nil {
			return nil, fmt.Errorf("invalid glob pattern %q: %w", pattern, err)
		}
	}

	return &Pattern{
		original: pattern,
		isGlob:   isGlob,
	}, nil
}

func (p *Pattern) match(namespace string) bool {
	if p.isGlob {
		// We validated this pattern in NewPattern, so this should never error.
		match, _ := filepath.Match(p.original, namespace)
		return match
	}
	return p.original == namespace
}

func (p *Pattern) String() string {
	return p.original
}

func ParsePatterns(patterns []string) ([]*Pattern, error) {
	if len(patterns) == 0 {
		return nil, nil
	}

	parsed := make([]*Pattern, 0, len(patterns))
	for _, pattern := range patterns {
		p, err := newPattern(pattern)
		if err != nil {
			return nil, fmt.Errorf("failed to parse pattern %q: %w", pattern, err)
		}
		parsed = append(parsed, p)
	}

	return parsed, nil
}

type NamespaceHandler interface {
	OnNamespaceAdded(namespace string) error
	OnNamespaceDeleted(namespace string) error
}

type InformerWatcher struct {
	clientset *kubernetes.Clientset
	factory   informers.SharedInformerFactory
	informer  cache.SharedIndexInformer
	lister    v1.NamespaceLister
	stopCh    chan struct{}
}

func NewInformerWatcher(clientset *kubernetes.Clientset) *InformerWatcher {
	factory := informers.NewSharedInformerFactory(clientset, 0)
	informer := factory.Core().V1().Namespaces().Informer()
	lister := factory.Core().V1().Namespaces().Lister()

	return &InformerWatcher{
		clientset: clientset,
		factory:   factory,
		informer:  informer,
		lister:    lister,
	}
}

func (w *InformerWatcher) Start(ctx context.Context, includePatterns, excludePatterns []*Pattern, handler NamespaceHandler) error {
	_, err := w.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %w", err)
	}

	w.stopCh = make(chan struct{})

	w.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			namespace := obj.(*corev1.Namespace)
			if w.shouldIncludeNamespace(namespace.Name, includePatterns, excludePatterns) {
				if err := handler.OnNamespaceAdded(namespace.Name); err != nil {
					fmt.Printf("Error handling namespace added %s: %v\n", namespace.Name, err)
				}
			}
		},
		DeleteFunc: func(obj any) {
			namespace := obj.(*corev1.Namespace)
			if w.shouldIncludeNamespace(namespace.Name, includePatterns, excludePatterns) {
				if err := handler.OnNamespaceDeleted(namespace.Name); err != nil {
					fmt.Printf("Error handling namespace deleted %s: %v\n", namespace.Name, err)
				}
			}
		},
	})

	go w.factory.Start(w.stopCh)

	if !cache.WaitForCacheSync(ctx.Done(), w.informer.HasSynced) {
		return fmt.Errorf("failed to sync namespace informer")
	}

	namespaces, err := w.lister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list existing namespaces: %w", err)
	}

	for _, ns := range namespaces {
		if w.shouldIncludeNamespace(ns.Name, includePatterns, excludePatterns) {
			if err := handler.OnNamespaceAdded(ns.Name); err != nil {
				return fmt.Errorf("error handling existing namespace %s: %w", ns.Name, err)
			}
		}
	}

	return nil
}

func (w *InformerWatcher) Stop() {
	if w.stopCh != nil {
		close(w.stopCh)
		w.stopCh = nil
	}
}

func (w *InformerWatcher) shouldIncludeNamespace(namespace string, includePatterns, excludePatterns []*Pattern) bool {
	included := len(includePatterns) == 0

	if len(includePatterns) > 0 {
		for _, pattern := range includePatterns {
			if pattern.match(namespace) {
				included = true
				break
			}
		}
	}

	if !included {
		return false
	}

	for _, pattern := range excludePatterns {
		if pattern.match(namespace) {
			return false
		}
	}

	return true
}

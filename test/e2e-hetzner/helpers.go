//go:build e2e_hetzner

package e2ehetzner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// streamLogs streams logs from every pod matching a deployment's selector.
// New pods (e.g. after leader changes) are picked up automatically.
func streamLogs(ctx context.Context, kubeconfig, namespace, deployment, prefix string) {
	go func() {
		for ctx.Err() == nil {
			if err := runStreamLogs(ctx, kubeconfig, namespace, deployment, prefix); err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("%s stream error: %v", prefix, err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
				log.Printf("%s reconnecting...", prefix)
			}
		}
	}()
}

func runStreamLogs(ctx context.Context, kubeconfig, namespace, deployment, prefix string) error {
	restCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return fmt.Errorf("building rest config: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("creating clientset: %w", err)
	}

	dep, err := clientset.AppsV1().Deployments(namespace).Get(ctx, deployment, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting deployment: %w", err)
	}

	selector := labelSelector(dep.Spec.Selector.MatchLabels)

	var mu sync.Mutex
	streaming := map[string]context.CancelFunc{}

	startStream := func(pod corev1.Pod) {
		mu.Lock()
		defer mu.Unlock()
		if _, ok := streaming[pod.Name]; ok {
			return
		}
		podCtx, podCancel := context.WithCancel(ctx)
		streaming[pod.Name] = podCancel
		podPrefix := fmt.Sprintf("%s/%s", prefix, pod.Name)
		go func() {
			defer func() {
				mu.Lock()
				delete(streaming, pod.Name)
				mu.Unlock()
			}()
			streamPodLogs(podCtx, clientset, namespace, pod.Name, podPrefix)
		}()
	}

	stopStream := func(podName string) {
		mu.Lock()
		defer mu.Unlock()
		if cancel, ok := streaming[podName]; ok {
			cancel()
			delete(streaming, podName)
		}
	}

	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return fmt.Errorf("listing pods: %w", err)
	}
	for _, pod := range pods.Items {
		startStream(pod)
	}

	watcher, err := clientset.CoreV1().Pods(namespace).Watch(ctx, metav1.ListOptions{
		LabelSelector: selector,
		ResourceVersion: pods.ResourceVersion,
	})
	if err != nil {
		return fmt.Errorf("watching pods: %w", err)
	}
	defer watcher.Stop()

	for event := range watcher.ResultChan() {
		pod, ok := event.Object.(*corev1.Pod)
		if !ok {
			continue
		}
		switch event.Type {
		case watch.Added, watch.Modified:
			startStream(*pod)
		case watch.Deleted:
			stopStream(pod.Name)
		}
	}
	return nil
}

func streamPodLogs(ctx context.Context, clientset kubernetes.Interface, namespace, podName, prefix string) {
	log.Printf("%s streaming started", prefix)
	stream, err := clientset.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		Follow:    true,
		TailLines: ptr.To(int64(0)),
	}).Stream(ctx)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("%s log stream error: %v", prefix, err)
		}
		return
	}
	defer stream.Close()

	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		log.Printf("%s %s", prefix, scanner.Text())
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		log.Printf("%s scanner error: %v", prefix, err)
	}
}

func labelSelector(labels map[string]string) string {
	var parts []string
	for k, v := range labels {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, ",")
}

func watchResource(ctx context.Context, c client.WithWatch, list client.ObjectList, prefix string) {
	go func() {
		for ctx.Err() == nil {
			if err := runWatch(ctx, c, list, prefix); err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("%s watch error: %v", prefix, err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
				log.Printf("%s reconnecting...", prefix)
			}
		}
	}()
}

func runWatch(ctx context.Context, c client.WithWatch, list client.ObjectList, prefix string) error {
	watcher, err := c.Watch(ctx, list)
	if err != nil {
		return fmt.Errorf("starting watch: %w", err)
	}
	defer watcher.Stop()

	log.Printf("%s watching started", prefix)
	for event := range watcher.ResultChan() {
		if event.Type == watch.Error {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("%s watch terminated by server, will reconnect", prefix)
			return nil
		}
		raw, err := json.Marshal(event.Object)
		if err != nil {
			log.Printf("%s %s (marshal error: %v)", prefix, event.Type, err)
			continue
		}
		log.Printf("%s %s %s", prefix, event.Type, raw)
	}
	return nil
}

func waitForDeploymentReady(ctx context.Context, c client.Client, namespace, name string, timeout time.Duration) error {
	deadline := time.After(timeout)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("deployment %s/%s not ready after %v", namespace, name, timeout)
		case <-time.After(10 * time.Second):
			var dep appsv1.Deployment
			if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &dep); err != nil {
				log.Printf("[deploy] waiting for deployment %s: %v", name, err)
				continue
			}
			if dep.Status.ReadyReplicas == *dep.Spec.Replicas {
				return nil
			}
			log.Printf("[deploy] %s: %d/%d replicas ready",
				name, dep.Status.ReadyReplicas, *dep.Spec.Replicas)
		}
	}
}


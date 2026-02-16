//go:build e2e_hetzner

package e2ehetzner

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	talosBootstrapTimeout = 10 * time.Minute
	nodeReadyTimeout      = 10 * time.Minute
	nodeReadyPollInterval = 10 * time.Second
	dialTimeout           = 5 * time.Second
)

type TalosCluster struct {
	Name         string
	TalosVersion string // e.g. "v1.11.0" — config contract compatibility
	K8sVersion   string // e.g. "v1.33.0" — Kubernetes version to bootstrap
	ServerIPs    []string
	ConfigDir    string // temp dir for generated configs
	TalosConfig  string // path to talosconfig file
	Kubeconfig   string // path to kubeconfig file
}

func NewTalosCluster(name string, talosVersion string, k8sVersion string, ips []string) (*TalosCluster, error) {
	dir, err := os.MkdirTemp("", "tuppr-e2e-talos-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	return &TalosCluster{
		Name:         name,
		TalosVersion: talosVersion,
		K8sVersion:   k8sVersion,
		ServerIPs:    ips,
		ConfigDir:    dir,
	}, nil
}

func (tc *TalosCluster) Bootstrap(ctx context.Context) error {
	if err := tc.genConfig(ctx); err != nil {
		return fmt.Errorf("generating config: %w", err)
	}

	for i, ip := range tc.ServerIPs {
		log.Printf("[talos] waiting for Talos API on node %d (%s)", i, ip)
		if err := tc.waitForTalosAPI(ctx, ip); err != nil {
			return fmt.Errorf("waiting for Talos API on node %d (%s): %w", i, ip, err)
		}
		log.Printf("[talos] applying config to node %d (%s)", i, ip)
		if err := tc.applyConfig(ctx, i, ip); err != nil {
			return fmt.Errorf("applying config to node %d (%s): %w", i, ip, err)
		}
	}

	log.Printf("[talos] bootstrapping cluster on %s", tc.ServerIPs[0])
	if err := tc.talosctlWithConfig(ctx, "bootstrap", "--nodes", tc.ServerIPs[0]); err != nil {
		return fmt.Errorf("bootstrapping: %w", err)
	}

	log.Printf("[talos] waiting for Kubernetes API to be ready")
	if err := tc.waitForKubernetesReady(ctx); err != nil {
		return fmt.Errorf("waiting for Kubernetes: %w", err)
	}

	log.Printf("[talos] fetching kubeconfig")
	if err := tc.fetchKubeconfig(ctx); err != nil {
		return fmt.Errorf("fetching kubeconfig: %w", err)
	}

	log.Printf("[talos] waiting for all nodes to be Ready")
	if err := tc.waitForNodesReady(ctx); err != nil {
		return fmt.Errorf("waiting for nodes ready: %w", err)
	}

	log.Printf("[talos] cluster bootstrap complete")
	return nil
}

func (tc *TalosCluster) Cleanup() {
	if tc.ConfigDir != "" {
		os.RemoveAll(tc.ConfigDir)
	}
}

func (tc *TalosCluster) genConfig(ctx context.Context) error {
	endpoint := fmt.Sprintf("https://%s:6443", tc.ServerIPs[0])

	patches := []map[string]any{
		{
			"cluster": map[string]any{
				"allowSchedulingOnControlPlanes": true,
			},
		},
		{
			"machine": map[string]any{
				"features": map[string]any{
					"kubernetesTalosAPIAccess": map[string]any{
						"enabled": true,
						"allowedRoles": []string{
							"os:admin",
						},
						"allowedKubernetesNamespaces": []string{
							"tuppr-system",
						},
					},
				},
			},
		},
	}

	args := []string{
		"gen", "config",
		tc.Name,
		endpoint,
		"--output", tc.ConfigDir,
		"--force",
		"--talos-version", tc.TalosVersion,
		"--kubernetes-version", tc.K8sVersion,
	}
	for _, p := range patches {
		pj, err := json.Marshal(p)
		if err != nil {
			return fmt.Errorf("marshaling patch: %w", err)
		}
		args = append(args, "--config-patch", string(pj))
	}

	if err := tc.talosctl(ctx, args...); err != nil {
		return err
	}

	tc.TalosConfig = filepath.Join(tc.ConfigDir, "talosconfig")

	endpointArgs := append([]string{"config", "endpoint"}, tc.ServerIPs...)
	if err := tc.talosctlWithConfig(ctx, endpointArgs...); err != nil {
		return fmt.Errorf("setting endpoints: %w", err)
	}
	if err := tc.talosctlWithConfig(ctx, "config", "node", tc.ServerIPs[0]); err != nil {
		return fmt.Errorf("setting default node: %w", err)
	}

	return nil
}

func (tc *TalosCluster) applyConfig(ctx context.Context, index int, ip string) error {
	configFile := filepath.Join(tc.ConfigDir, "controlplane.yaml")

	hostnamePatch := fmt.Sprintf(`{"machine": {"network": {"hostname": "%s-node-%d"}}}`, tc.Name, index)

	return tc.talosctl(ctx,
		"apply-config",
		"--nodes", ip,
		"--file", configFile,
		"--insecure",
		"--config-patch", hostnamePatch,
	)
}

func (tc *TalosCluster) waitForKubernetesReady(ctx context.Context) error {
	deadline, cancel := context.WithTimeout(ctx, talosBootstrapTimeout)
	defer cancel()

	endpoints := strings.Join(tc.ServerIPs, ",")
	return tc.talosctlWithConfig(deadline,
		"health",
		"--nodes", tc.ServerIPs[0],
		"--control-plane-nodes", endpoints,
		"--wait-timeout", talosBootstrapTimeout.String(),
	)
}

func (tc *TalosCluster) fetchKubeconfig(ctx context.Context) error {
	kubeconfigPath := filepath.Join(tc.ConfigDir, "kubeconfig")

	if err := tc.talosctlWithConfig(ctx,
		"kubeconfig",
		"--nodes", tc.ServerIPs[0],
		"--force",
		kubeconfigPath,
	); err != nil {
		return err
	}

	tc.Kubeconfig = kubeconfigPath
	return nil
}

func (tc *TalosCluster) waitForTalosAPI(ctx context.Context, ip string) error {
	deadline := time.After(connectTimeout)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("Talos API not available at %s:50000 after %v", ip, connectTimeout)
		case <-time.After(retryInterval):
			conn, err := net.DialTimeout("tcp", ip+":50000", dialTimeout)
			if err == nil {
				conn.Close()
				return nil
			}
		}
	}
}

func (tc *TalosCluster) waitForNodesReady(ctx context.Context) error {
	deadline := time.After(nodeReadyTimeout)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("nodes not ready after %v", nodeReadyTimeout)
		case <-time.After(nodeReadyPollInterval):
			ready, err := tc.allNodesReady(ctx)
			if err != nil {
				log.Printf("[talos] checking node readiness: %v", err)
				continue
			}
			if ready {
				return nil
			}
		}
	}
}

func (tc *TalosCluster) allNodesReady(ctx context.Context) (bool, error) {
	restCfg, err := clientcmd.BuildConfigFromFlags("", tc.Kubeconfig)
	if err != nil {
		return false, err
	}
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return false, err
	}
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}
	if len(nodes.Items) < len(tc.ServerIPs) {
		return false, nil
	}
	for _, node := range nodes.Items {
		ready := false
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}
		if !ready {
			return false, nil
		}
	}
	return true, nil
}

func (tc *TalosCluster) talosctl(ctx context.Context, args ...string) error {
	log.Printf("[talos] talosctl %s", strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, "talosctl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("talosctl %s: %w", args[0], err)
	}
	return nil
}

func (tc *TalosCluster) talosctlWithConfig(ctx context.Context, args ...string) error {
	fullArgs := append([]string{"--talosconfig", tc.TalosConfig}, args...)
	return tc.talosctl(ctx, fullArgs...)
}

func (tc *TalosCluster) talosctlOutput(ctx context.Context, args ...string) (string, error) {
	fullArgs := append([]string{"--talosconfig", tc.TalosConfig}, args...)
	cmd := exec.CommandContext(ctx, "talosctl", fullArgs...)
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("talosctl %s: %w\nstderr: %s", args[0], err, string(ee.Stderr))
		}
		return "", fmt.Errorf("talosctl %s: %w", args[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (tc *TalosCluster) NodeVersions(ctx context.Context) (map[string]string, error) {
	versions := make(map[string]string, len(tc.ServerIPs))
	for _, ip := range tc.ServerIPs {
		out, err := tc.talosctlOutput(ctx, "version", "--nodes", ip, "--short")
		if err != nil {
			return nil, fmt.Errorf("getting version for %s: %w", ip, err)
		}
		// talosctl version --short outputs:
		//   Client:
		//     Tag: v1.12.4
		//   Server:
		//     Tag: v1.11.0
		// We want the server tag (the last "Tag:" line).
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Tag:") {
				versions[ip] = strings.TrimSpace(strings.TrimPrefix(line, "Tag:"))
			}
		}
		if _, ok := versions[ip]; !ok {
			return nil, fmt.Errorf("could not parse Talos version for %s from:\n%s", ip, out)
		}
	}
	return versions, nil
}

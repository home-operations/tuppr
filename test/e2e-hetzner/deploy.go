//go:build e2e_hetzner

package e2ehetzner

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	controllerNamespace    = "tuppr-system"
	helmReleaseName        = "tuppr"
	controllerReadyTimeout = 5 * time.Minute
)

func BuildAndPushImage(ctx context.Context, cfg *Config, runID string) (string, error) {
	if cfg.ControllerImage != "" {
		log.Printf("[deploy] using pre-built image: %s", cfg.ControllerImage)
		return cfg.ControllerImage, nil
	}

	image := fmt.Sprintf("ttl.sh/tuppr-%s:2h", runID)
	log.Printf("[deploy] building image: %s", image)

	repoRoot, err := repoRootDir()
	if err != nil {
		return "", err
	}

	buildCmd := exec.CommandContext(ctx, "docker", "build", "-t", image, ".")
	buildCmd.Dir = repoRoot
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("docker build: %w", err)
	}

	log.Printf("[deploy] pushing image: %s", image)
	pushCmd := exec.CommandContext(ctx, "docker", "push", image)
	pushCmd.Stdout = os.Stdout
	pushCmd.Stderr = os.Stderr
	if err := pushCmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("docker push: %w", err)
	}

	return image, nil
}

func DeployController(ctx context.Context, c client.Client, kubeconfig, image string) error {
	repoRoot, err := repoRootDir()
	if err != nil {
		return err
	}

	log.Printf("[deploy] applying CRDs")
	if err := applyCRDs(ctx, c, filepath.Join(repoRoot, "config", "crd", "bases")); err != nil {
		return fmt.Errorf("applying CRDs: %w", err)
	}

	log.Printf("[deploy] creating namespace %s", controllerNamespace)
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: controllerNamespace,
			Labels: map[string]string{
				"pod-security.kubernetes.io/enforce": "privileged",
				"pod-security.kubernetes.io/warn":    "privileged",
			},
		},
	}
	if err := c.Create(ctx, ns); err != nil {
		return fmt.Errorf("creating namespace: %w", err)
	}

	ref, err := name.ParseReference(image)
	if err != nil {
		return fmt.Errorf("parsing image reference %q: %w", image, err)
	}
	repo := ref.Context().String()
	tag := ref.Identifier()

	chartPath := filepath.Join(repoRoot, "charts", "tuppr")
	log.Printf("[deploy] helm install %s (repo=%s, tag=%s)", helmReleaseName, repo, tag)

	helmArgs := []string{
		"install", helmReleaseName, chartPath,
		"--namespace", controllerNamespace,
		"--set", fmt.Sprintf("image.repository=%s", repo),
		"--set", fmt.Sprintf("image.tag=%s", tag),
		"--set", "image.pullPolicy=Always",
		"--set", "replicaCount=2",
		"--wait",
		"--timeout", controllerReadyTimeout.String(),
		"--kubeconfig", kubeconfig,
	}

	cmd := exec.CommandContext(ctx, "helm", helmArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("helm install: %w", err)
	}

	log.Printf("[deploy] controller deployed successfully")
	return nil
}

func WaitForController(ctx context.Context, c client.Client) error {
	return waitForDeploymentReady(ctx, c, controllerNamespace, "tuppr", controllerReadyTimeout)
}

func applyCRDs(ctx context.Context, c client.Client, crdDir string) error {
	entries, err := os.ReadDir(crdDir)
	if err != nil {
		return fmt.Errorf("reading CRD dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(crdDir, e.Name()))
		if err != nil {
			return fmt.Errorf("reading %s: %w", e.Name(), err)
		}
		crd := &apiextensionsv1.CustomResourceDefinition{}
		if err := yaml.NewYAMLOrJSONDecoder(strings.NewReader(string(data)), len(data)).Decode(crd); err != nil {
			return fmt.Errorf("decoding %s: %w", e.Name(), err)
		}
		log.Printf("[deploy] applying CRD %s", crd.Name)
		if err := c.Create(ctx, crd); err != nil {
			return fmt.Errorf("creating CRD %s: %w", crd.Name, err)
		}
	}
	return nil
}

func repoRootDir() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("finding repo root: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

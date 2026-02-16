//go:build e2e_hetzner

package e2ehetzner

import (
	"fmt"
	"os"
	"os/exec"
)

type Config struct {
	HCloudToken      string
	ServerType       string
	Location         string
	TalosFromVersion string
	TalosToVersion   string
	K8sFromVersion   string
	K8sToVersion     string
	ControllerImage  string // if set, skip build and use this image
}

func LoadConfig() (*Config, error) {
	cfg := &Config{
		HCloudToken:      os.Getenv("HCLOUD_TOKEN"),
		ServerType:       envOrDefault("HCLOUD_SERVER_TYPE", "cx23"),
		Location:         envOrDefault("HCLOUD_LOCATION", "nbg1"),
		TalosFromVersion: envOrDefault("TALOS_FROM_VERSION", "v1.11.0"),
		TalosToVersion:   envOrDefault("TALOS_TO_VERSION", "v1.12.4"),
		K8sFromVersion:   envOrDefault("K8S_FROM_VERSION", "v1.34.0"),
		K8sToVersion:     envOrDefault("K8S_TO_VERSION", "v1.35.0"),
		ControllerImage:  os.Getenv("CONTROLLER_IMAGE"),
	}

	if cfg.HCloudToken == "" {
		return nil, fmt.Errorf("HCLOUD_TOKEN environment variable is required")
	}

	return cfg, nil
}

func CheckPrerequisites() error {
	tools := []string{"talosctl", "helm", "docker"}
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("required tool %q not found in PATH", tool)
		}
	}
	return nil
}

func envOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

package controller

import (
	"fmt"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
)

// GetNodeInternalIP retrieves the InternalIP of a Kubernetes node
func GetNodeInternalIP(node *corev1.Node) (string, error) {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address, nil
		}
	}
	return "", fmt.Errorf("no InternalIP found for node %q", node.Name)
}

// GenerateSafeJobName generates a safe Kubernetes Job name
// It ensures the name is within Kubernetes' 63 character limit
// by truncating the identifier if necessary and appending a unique suffix.
func GenerateSafeJobName(prefix, identifier string) string {
	jobID := uuid.New().String()[:8]
	const maxLength = 57

	prefixLen := len(prefix) + len(jobID) + 2
	available := maxLength - prefixLen

	if available <= 0 {
		return fmt.Sprintf("%s-%s", prefix, jobID)
	}

	identifierLen := min(len(identifier), available)
	if identifierLen > 0 {
		return fmt.Sprintf("%s-%s-%s", prefix, identifier[:identifierLen], jobID)
	}

	return fmt.Sprintf("%s-%s", prefix, jobID)
}

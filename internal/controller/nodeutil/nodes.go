package nodeutil

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	corev1 "k8s.io/api/core/v1"
)

// Now provides time functionality for testing
type Now interface {
	Now() time.Time
}

// Clock implements Now interface
type Clock struct{}

func (Clock) Now() time.Time {
	return time.Now()
}

func GetNodeIP(node *corev1.Node) (string, error) {
	externalIP := ""

	for _, addr := range node.Status.Addresses {
		switch addr.Type {
		case corev1.NodeInternalIP:
			return addr.Address, nil
		case corev1.NodeExternalIP:
			externalIP = addr.Address
		}
	}
	if externalIP != "" {
		return externalIP, nil
	}

	return "", fmt.Errorf("no InternalIP or ExternalIP found for node %q", node.Name)
}

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

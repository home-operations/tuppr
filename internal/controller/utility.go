package controller

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/google/uuid"
	upgradev1alpha1 "github.com/home-operations/talup/api/v1alpha1"
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

// GetTalosSchematicAndVersion extracts the Talos schematic and version from the image string
func GetTalosSchematicAndVersion(image string) (version, schematic string) {
	idx := strings.LastIndex(image, ":")
	if idx == -1 || idx == len(image)-1 {
		return "", ""
	}
	version = image[idx+1:]
	imagePath := image[:idx]

	if strings.Contains(imagePath, "factory.talos.dev") {
		schematic = path.Base(imagePath)
	}
	return version, schematic
}

// GetTalosSchematic extracts the Talos schematic from node annotations
func GetTalosSchematic(node *corev1.Node) string {
	if node.Annotations == nil {
		return ""
	}
	return node.Annotations[SchematicAnnotation]
}

// GetTalosVersion extracts the Talos version from the OS image string
func GetTalosVersion(osImage string) string {
	// osImage format: "Talos (v1.11.1)"
	// Extract the version part between parentheses
	start := strings.Index(osImage, "(")
	end := strings.Index(osImage, ")")

	if start == -1 || end == -1 || start >= end {
		return ""
	}

	version := osImage[start+1 : end]
	return strings.TrimSpace(version)
}

// GenerateSafeJobName creates a Kubernetes-compliant job name that ensures
// both Job names and generated Pod names stay under the 63-character limit
func GenerateSafeJobName(upgradeName, nodeName string) string {
	jobID := uuid.New().String()[:8]

	// Kubernetes generates Pod names as: {job-name}-{5-char-suffix}
	// Reserve 6 characters for Pod suffix
	const maxJobNameLength = 63 - 6 // 57 characters max for Job name

	// Use strings.Builder for efficient string construction
	var nameBuilder strings.Builder
	nameBuilder.WriteString("talup-")

	// Calculate remaining space after prefix and suffix
	fixedLength := nameBuilder.Len() + 1 + len(jobID) // +1 for final dash
	availableLength := maxJobNameLength - fixedLength

	// Use min() from Go 1.21+
	nodeLen := min(len(nodeName), availableLength*2/3)             // Prioritize node name
	upgradeLen := min(len(upgradeName), availableLength-nodeLen-1) // -1 for dash

	// Handle edge cases with max()
	upgradeLen = max(0, upgradeLen)
	if upgradeLen == 0 {
		nodeLen = min(len(nodeName), availableLength)
	}

	// Build name efficiently
	if upgradeLen > 0 {
		nameBuilder.WriteString(upgradeName[:upgradeLen])
		nameBuilder.WriteString("-")
	}
	nameBuilder.WriteString(nodeName[:nodeLen])
	nameBuilder.WriteString("-")
	nameBuilder.WriteString(jobID)

	result := nameBuilder.String()

	// Final safety check
	if len(result) > maxJobNameLength {
		// Emergency fallback
		return fmt.Sprintf("talup-%s", jobID)
	}

	return result
}

// ContainsNode checks if a node name is in the list of nodes
func ContainsNode(nodeName string, nodes []string) bool {
	return slices.Contains(nodes, nodeName)
}

// ContainsFailedNode checks if a node name is in the list of failed nodes
func ContainsFailedNode(nodeName string, failedNodes []upgradev1alpha1.NodeUpgradeStatus) bool {
	return slices.ContainsFunc(failedNodes, func(n upgradev1alpha1.NodeUpgradeStatus) bool {
		return n.NodeName == nodeName
	})
}

package controller

import (
	"fmt"
	"path"
	"slices"
	"strings"

	upgradev1alpha1 "github.com/home-operations/talup/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

func GetNodeInternalIP(node *corev1.Node) (string, error) {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address, nil
		}
	}
	return "", fmt.Errorf("no InternalIP found for node %q", node.Name)
}

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

func GetTalosSchematic(node *corev1.Node) string {
	if node.Annotations == nil {
		return ""
	}
	return node.Annotations[SchematicAnnotation]
}

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

func ContainsNode(nodeName string, nodes []string) bool {
	return slices.Contains(nodes, nodeName)
}

func ContainsFailedNode(nodeName string, failedNodes []upgradev1alpha1.NodeUpgradeStatus) bool {
	return slices.ContainsFunc(failedNodes, func(n upgradev1alpha1.NodeUpgradeStatus) bool {
		return n.NodeName == nodeName
	})
}

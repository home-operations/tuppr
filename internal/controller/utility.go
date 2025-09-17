package controller

import (
	"fmt"
	"slices"
	"strings"

	"github.com/distribution/reference"
	upgradev1alpha1 "github.com/home-operations/talup/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

func GetNodeInternalIP(node *corev1.Node) (string, error) {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address, nil
		}
	}
	return "", fmt.Errorf("no internal IP found for node %s", node.Name)
}

func ExtractVersionAndSchematic(image string) (version, schematic string) {
	imagePath, version, found := strings.Cut(image, ":")
	if !found {
		return "", ""
	}

	if strings.Contains(imagePath, "factory.talos.dev") {
		pathParts := strings.Split(imagePath, "/")
		if len(pathParts) >= 3 {
			schematic = pathParts[len(pathParts)-1]
		}
	}

	return version, schematic
}

func GetSchematicFromNode(node *corev1.Node) string {
	if node.Annotations == nil {
		return ""
	}
	return node.Annotations[SchematicAnnotation]
}

func ExtractSchematicFromMachineConfig(installImage string) (string, error) {
	ref, err := reference.ParseAnyReference(installImage)
	if err != nil {
		return "", fmt.Errorf("failed to parse install image reference: %w", err)
	}

	namedRef, ok := ref.(reference.Named)
	if !ok {
		return "", fmt.Errorf("not a named reference")
	}

	imageName := namedRef.Name()
	if strings.Contains(imageName, "factory.talos.dev") {
		parts := strings.Split(imageName, "/")
		if len(parts) >= 3 {
			return parts[len(parts)-1], nil
		}
	}

	return "", fmt.Errorf("no schematic found in image: %s", installImage)
}

func IsNodeInList(nodeName string, nodeList []string) bool {
	return slices.Contains(nodeList, nodeName)
}

func IsNodeInFailedList(nodeName string, failedNodes []upgradev1alpha1.NodeUpgradeStatus) bool {
	return slices.ContainsFunc(failedNodes, func(node upgradev1alpha1.NodeUpgradeStatus) bool {
		return node.NodeName == nodeName
	})
}

func ExtractVersionFromOSImage(osImage string) string {
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

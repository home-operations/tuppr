package nodeutil

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/home-operations/tuppr/internal/constants"
)

// Talos sets Node.status.nodeInfo.osImage to e.g. "Talos (v1.7.5)".
var talosVersionRegexp = regexp.MustCompile(`v\d+\.\d+\.\d+[^\s)]*`)

// TalosVersionFromOSImage returns the Talos version in a Node's osImage, or ""
// when it names none.
func TalosVersionFromOSImage(osImage string) string {
	return talosVersionRegexp.FindString(osImage)
}

// TargetTalosVersion returns the Talos version the node is upgraded to: its
// version annotation when set, otherwise the plan's version.
func TargetTalosVersion(node *corev1.Node, planVersion string) string {
	if v := node.Annotations[constants.VersionAnnotation]; v != "" {
		return v
	}
	return planVersion
}

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

// ControlPlaneEndpointIPs returns the IPs of Ready control-plane nodes, sorted by name,
// for use as Talos API endpoints that don't depend on cluster DNS. Returns nil on a list
// failure or when none are Ready.
func ControlPlaneEndpointIPs(ctx context.Context, reader client.Reader, controlPlaneLabel string) []string {
	nodes := &corev1.NodeList{}
	if err := reader.List(ctx, nodes, client.MatchingLabels{controlPlaneLabel: ""}); err != nil {
		return nil
	}

	slices.SortFunc(nodes.Items, func(a, b corev1.Node) int {
		return strings.Compare(a.Name, b.Name)
	})

	var ips []string
	for i := range nodes.Items {
		node := &nodes.Items[i]
		if !isNodeReady(node) {
			continue
		}
		if ip, err := GetNodeIP(node); err == nil {
			ips = append(ips, ip)
		}
	}
	return ips
}

func isNodeReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
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

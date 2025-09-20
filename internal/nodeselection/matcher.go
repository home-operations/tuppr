package nodeselection

import (
	"slices"

	upgradev1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// Matcher handles node selection logic
type Matcher struct{}

func NewMatcher() *Matcher {
	return &Matcher{}
}

// GetMatchingNodes returns nodes that match the TalosUpgrade's node selector
func (m *Matcher) GetMatchingNodes(allNodes []corev1.Node, talos *upgradev1alpha1.TalosUpgrade) []corev1.Node {
	if len(talos.Spec.Target.NodeSelectorExprs) == 0 {
		return allNodes // No selector means all nodes
	}

	selector := &corev1.NodeSelector{
		NodeSelectorTerms: []corev1.NodeSelectorTerm{{
			MatchExpressions: talos.Spec.Target.NodeSelectorExprs,
		}},
	}

	return slices.DeleteFunc(slices.Clone(allNodes), func(node corev1.Node) bool {
		return !m.NodeMatchesSelector(&node, selector)
	})
}

// NodeMatchesSelector checks if a node matches the given node selector
func (m *Matcher) NodeMatchesSelector(node *corev1.Node, selector *corev1.NodeSelector) bool {
	return len(selector.NodeSelectorTerms) == 0 || // Empty selector matches all
		slices.ContainsFunc(selector.NodeSelectorTerms, func(term corev1.NodeSelectorTerm) bool {
			return m.nodeMatchesTerm(node, &term)
		})
}

// nodeMatchesTerm checks if a node matches all requirements in a term
func (m *Matcher) nodeMatchesTerm(node *corev1.Node, term *corev1.NodeSelectorTerm) bool {
	matchesAllExpressions := !slices.ContainsFunc(term.MatchExpressions, func(expr corev1.NodeSelectorRequirement) bool {
		return !m.nodeMatchesExpression(node, &expr)
	})

	matchesAllFields := !slices.ContainsFunc(term.MatchFields, func(field corev1.NodeSelectorRequirement) bool {
		return !m.nodeMatchesFieldExpression(node, &field)
	})

	return matchesAllExpressions && matchesAllFields
}

// nodeMatchesExpression checks if a node matches a single node selector requirement
func (m *Matcher) nodeMatchesExpression(node *corev1.Node, expr *corev1.NodeSelectorRequirement) bool {
	nodeValue, exists := node.Labels[expr.Key]

	switch expr.Operator {
	case corev1.NodeSelectorOpIn:
		return exists && slices.Contains(expr.Values, nodeValue)

	case corev1.NodeSelectorOpNotIn:
		return !exists || !slices.Contains(expr.Values, nodeValue)

	case corev1.NodeSelectorOpExists:
		return exists

	case corev1.NodeSelectorOpDoesNotExist:
		return !exists

	case corev1.NodeSelectorOpGt:
		return exists && len(expr.Values) > 0 && nodeValue > expr.Values[0]

	case corev1.NodeSelectorOpLt:
		return exists && len(expr.Values) > 0 && nodeValue < expr.Values[0]

	default:
		return false
	}
}

// nodeMatchesFieldExpression checks if a node matches a field selector requirement
func (m *Matcher) nodeMatchesFieldExpression(node *corev1.Node, expr *corev1.NodeSelectorRequirement) bool {
	fieldMap := map[string]string{
		"metadata.name":      node.Name,
		"metadata.namespace": node.Namespace,
	}

	nodeValue, exists := fieldMap[expr.Key]
	if !exists {
		return expr.Operator == corev1.NodeSelectorOpDoesNotExist
	}

	return m.matchesStringValue(nodeValue, expr)
}

// matchesStringValue checks if a string value matches the requirements
func (m *Matcher) matchesStringValue(value string, expr *corev1.NodeSelectorRequirement) bool {
	switch expr.Operator {
	case corev1.NodeSelectorOpIn:
		return slices.Contains(expr.Values, value)

	case corev1.NodeSelectorOpNotIn:
		return !slices.Contains(expr.Values, value)

	case corev1.NodeSelectorOpExists:
		return true

	case corev1.NodeSelectorOpDoesNotExist:
		return false

	default:
		return false
	}
}

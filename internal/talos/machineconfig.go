package talos

// Raw, version-agnostic machine config handling.
//
// A node's machine config can carry document kinds newer than any machinery this
// binary links (a 1.13 tuppr meeting 1.14's DiscoveryIdentityConfig or
// BGPPeerConfig), and machinery's decoder hard-errors on unknown kinds.
// Reading and patching the install image therefore never decodes the whole
// config: documents are split textually, only the two kinds that can carry an
// install image are parsed, and every other document passes through byte for
// byte.

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// unattendedInstallKind is the 1.14+ document that owns the installer image.
// Its presence also means the v1alpha1 .machine.install section is rejected by
// the node, so it decides which document a patch may touch.
const unattendedInstallKind = "UnattendedInstallConfig"

// splitDocuments splits a multi-document YAML stream on standalone `---`
// separator lines, dropping empty segments and preserving each document's text.
func splitDocuments(raw string) []string {
	var docs []string
	for _, doc := range strings.Split("\n"+raw, "\n---") {
		doc = strings.TrimPrefix(doc, "\n")
		if strings.TrimSpace(doc) == "" {
			continue
		}
		docs = append(docs, doc)
	}
	return docs
}

// docProbe reads just enough of a document to classify it: typed documents have
// apiVersion/kind, the legacy v1alpha1 document has `version: v1alpha1` and no
// kind.
type docProbe struct {
	Version string `yaml:"version"`
	Kind    string `yaml:"kind"`
}

func probeDocument(doc string) docProbe {
	var probe docProbe
	// A document that does not even parse as YAML is left to the node to judge.
	_ = yaml.Unmarshal([]byte(doc), &probe)
	return probe
}

type unattendedInstallDoc struct {
	Installer struct {
		Image string `yaml:"image"`
	} `yaml:"installer"`
}

type v1alpha1InstallDoc struct {
	Machine struct {
		Install struct {
			Image string `yaml:"image"`
		} `yaml:"install"`
	} `yaml:"machine"`
}

// installImageFromConfig returns the node's installer image: the
// UnattendedInstall document's when present (Talos 1.14+), the legacy
// .machine.install.image otherwise.
func installImageFromConfig(raw string) (string, error) {
	var legacy string

	for _, doc := range splitDocuments(raw) {
		switch probe := probeDocument(doc); {
		case probe.Kind == unattendedInstallKind:
			var ui unattendedInstallDoc
			if err := yaml.Unmarshal([]byte(doc), &ui); err != nil {
				return "", fmt.Errorf("failed to parse %s document: %w", unattendedInstallKind, err)
			}
			if ui.Installer.Image != "" {
				return ui.Installer.Image, nil
			}
		case probe.Kind == "" && probe.Version == "v1alpha1":
			var v1 v1alpha1InstallDoc
			if err := yaml.Unmarshal([]byte(doc), &v1); err != nil {
				return "", fmt.Errorf("failed to parse v1alpha1 document: %w", err)
			}
			legacy = v1.Machine.Install.Image
		}
	}

	if legacy == "" {
		return "", fmt.Errorf("no install image in the machine config")
	}
	return legacy, nil
}

// setInstallImage returns the config with the installer image replaced, editing
// only the document that owns it and passing every other document through
// unchanged. The UnattendedInstall document wins when present, because a node
// carrying it rejects any .machine.install section applied alongside.
func setInstallImage(raw, image string) (string, error) {
	docs := splitDocuments(raw)

	target := -1
	var path []string
	for i, doc := range docs {
		switch probe := probeDocument(doc); {
		case probe.Kind == unattendedInstallKind:
			target, path = i, []string{"installer", "image"}
		case probe.Kind == "" && probe.Version == "v1alpha1" && target == -1:
			target, path = i, []string{"machine", "install", "image"}
		}
	}
	if target == -1 {
		return "", fmt.Errorf("no document owns the install image in the machine config")
	}

	edited, err := setYAMLPath(docs[target], path, image)
	if err != nil {
		return "", err
	}
	docs[target] = edited

	return strings.Join(docs, "\n---\n"), nil
}

// setYAMLPath sets a scalar at a mapping path inside one YAML document,
// creating intermediate mappings as needed and preserving the rest of the
// document (comments included) via the yaml.Node representation.
func setYAMLPath(doc string, path []string, value string) (string, error) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(doc), &root); err != nil {
		return "", fmt.Errorf("failed to parse document for patching: %w", err)
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return "", fmt.Errorf("unexpected document structure")
	}

	node := root.Content[0]
	for _, key := range path[:len(path)-1] {
		node = childMapping(node, key)
		if node == nil {
			return "", fmt.Errorf("cannot descend into %q", key)
		}
	}

	leaf := path[len(path)-1]
	for i := 0; i < len(node.Content)-1; i += 2 {
		if node.Content[i].Value == leaf {
			node.Content[i+1] = scalarNode(value)
			out, err := yaml.Marshal(&root)
			return string(out), err
		}
	}
	node.Content = append(node.Content, scalarNode(leaf), scalarNode(value))

	out, err := yaml.Marshal(&root)
	return string(out), err
}

func childMapping(node *yaml.Node, key string) *yaml.Node {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content)-1; i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	child := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	node.Content = append(node.Content, scalarNode(key), child)
	return child
}

func scalarNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

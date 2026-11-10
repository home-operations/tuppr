package talos

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A 1.13-style config: single v1alpha1 document, install image in
// .machine.install.image.
const config113 = `version: v1alpha1
machine:
  install:
    disk: /dev/vda
    image: factory.talos.dev/installer/abc:v1.13.5
cluster:
  clusterName: test
`

// A 1.14-style config: the v1alpha1 document has no .machine.install, the
// installer image lives in the UnattendedInstall document, and the stream
// carries typed documents — including kinds no machinery may know yet
// (BGPPeerConfig is real: seen on a beta node in the wild).
const config114 = `version: v1alpha1
machine:
  kubelet:
    image: ghcr.io/siderolabs/kubelet:v1.37.0
cluster:
  clusterName: test
---
apiVersion: v1alpha1
kind: UnattendedInstallConfig
installer:
  image: factory.talos.dev/metal-installer/abc:v1.14.0-beta.0
provisioning:
  diskSelector:
    match: system_disk
---
apiVersion: v1alpha1
kind: DiscoveryIdentityConfig
identity: some-identity
---
apiVersion: v1alpha1
kind: BGPPeerConfig
name: peer-1
peerAddress: 10.0.0.254
`

func TestInstallImageFromConfig(t *testing.T) {
	t.Run("1.13 v1alpha1 config", func(t *testing.T) {
		image, err := installImageFromConfig(config113)
		require.NoError(t, err)
		assert.Equal(t, "factory.talos.dev/installer/abc:v1.13.5", image)
	})

	t.Run("1.14 multi-document config with unknown kinds", func(t *testing.T) {
		image, err := installImageFromConfig(config114)
		require.NoError(t, err)
		assert.Equal(t, "factory.talos.dev/metal-installer/abc:v1.14.0-beta.0", image)
	})

	t.Run("no install image anywhere", func(t *testing.T) {
		_, err := installImageFromConfig("version: v1alpha1\nmachine: {}\n")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no install image")
	})

	// The UnattendedInstall document wins over a leftover legacy field, matching
	// the node's own precedence.
	t.Run("document wins over legacy field", func(t *testing.T) {
		raw := "version: v1alpha1\nmachine:\n  install:\n    image: old\n---\napiVersion: v1alpha1\nkind: UnattendedInstallConfig\ninstaller:\n  image: new\n"
		image, err := installImageFromConfig(raw)
		require.NoError(t, err)
		assert.Equal(t, "new", image)
	})
}

func TestSetInstallImage(t *testing.T) {
	t.Run("1.13 config edits machine.install.image", func(t *testing.T) {
		patched, err := setInstallImage(config113, "factory.talos.dev/installer/abc:v1.14.0")
		require.NoError(t, err)

		image, err := installImageFromConfig(patched)
		require.NoError(t, err)
		assert.Equal(t, "factory.talos.dev/installer/abc:v1.14.0", image)

		// Only the image line changes; the rest keeps its layout.
		assert.Equal(t, strings.Replace(config113, "v1.13.5", "v1.14.0", 1), patched)
	})

	t.Run("1.14 config edits the UnattendedInstall document only", func(t *testing.T) {
		patched, err := setInstallImage(config114, "factory.talos.dev/metal-installer/abc:v1.14.0-beta.1")
		require.NoError(t, err)

		image, err := installImageFromConfig(patched)
		require.NoError(t, err)
		assert.Equal(t, "factory.talos.dev/metal-installer/abc:v1.14.0-beta.1", image)

		// Only the image line changes: no .machine.install appears (a 1.14 node
		// rejects it alongside the document) and every other document, unknown
		// kinds included, passes through byte for byte.
		assert.Equal(t, strings.Replace(config114, "v1.14.0-beta.0", "v1.14.0-beta.1", 1), patched)
	})

	t.Run("edited document keeps its comments", func(t *testing.T) {
		for name, tc := range map[string]struct{ in, want string }{
			"v1alpha1": {
				in: `# managed by hand
version: v1alpha1
machine:
  # install section
  install:
    disk: /dev/vda # boot disk
    image: factory.talos.dev/installer/abc:v1.13.5 # pinned
cluster:
  clusterName: test
`,
				want: `# managed by hand
version: v1alpha1
machine:
  # install section
  install:
    disk: /dev/vda # boot disk
    image: img:new # pinned
cluster:
  clusterName: test
`,
			},
			"UnattendedInstallConfig": {
				in: `apiVersion: v1alpha1
kind: UnattendedInstallConfig
# installer section
installer:
  image: factory.talos.dev/metal-installer/abc:v1.14.0 # pinned
provisioning:
  diskSelector:
    match: system_disk # first disk
`,
				want: `apiVersion: v1alpha1
kind: UnattendedInstallConfig
# installer section
installer:
  image: img:new # pinned
provisioning:
  diskSelector:
    match: system_disk # first disk
`,
			},
		} {
			t.Run(name, func(t *testing.T) {
				patched, err := setInstallImage(tc.in, "img:new")
				require.NoError(t, err)
				assert.Equal(t, tc.want, patched)
			})
		}
	})

	t.Run("round-trips through repeated patches", func(t *testing.T) {
		once, err := setInstallImage(config114, "img:one")
		require.NoError(t, err)
		twice, err := setInstallImage(once, "img:two")
		require.NoError(t, err)

		image, err := installImageFromConfig(twice)
		require.NoError(t, err)
		assert.Equal(t, "img:two", image)
		assert.Equal(t, strings.Count(config114, "---"), strings.Count(twice, "---"))
	})

	t.Run("no owning document", func(t *testing.T) {
		_, err := setInstallImage("apiVersion: v1alpha1\nkind: BGPPeerConfig\nname: x\n", "img")
		require.Error(t, err)
	})
}

func TestSplitDocuments(t *testing.T) {
	docs := splitDocuments(config114)
	assert.Len(t, docs, 4)

	// A leading separator or trailing newline changes nothing.
	docs = splitDocuments("---\n" + config114 + "\n")
	assert.Len(t, docs, 4)
}

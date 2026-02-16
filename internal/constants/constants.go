package constants

// Default image constants
const (
	DefaultTalosctlImage = "ghcr.io/siderolabs/talosctl"
	DefaultTalosctlTag   = "latest"
)

// Annotation keys
const (
	ResetAnnotation   = "tuppr.home-operations.com/reset"
	SuspendAnnotation = "tuppr.home-operations.com/suspend"
)

const (
	// Override annotations
	SchematicAnnotation = "tuppr.home-operations.com/schematic"
	VersionAnnotation   = "tuppr.home-operations.com/version"

	// Default factory URL for schematic construction
	DefaultFactoryURL = "factory.talos.dev/installer"
)

// Node label constants
const (
	NodeUpgradingLabel = "tuppr.home-operations.com/upgrading"
)

// Talos config secret constants
const (
	TalosSecretName = "talosconfig"
	TalosSecretKey  = "config"
)

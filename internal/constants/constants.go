package constants

// Default image constants
const (
	DefaultTalosctlImage = "ghcr.io/siderolabs/talosctl"
	GenericInstallerRepo = "ghcr.io/siderolabs/installer"
)

// Annotation keys
const (
	ResetAnnotation   = "tuppr.home-operations.com/reset"
	SuspendAnnotation = "tuppr.home-operations.com/suspend"
)

const (
	// Override annotations
	VersionAnnotation    = "tuppr.home-operations.com/version"
	FactoryURLAnnotation = "tuppr.home-operations.com/factory-url"
	SchematicAnnotation  = "tuppr.home-operations.com/schematic"
)

// Node label constants
const (
	NodeUpgradingLabel = "tuppr.home-operations.com/upgrading"
)

// Node taint constants
const (
	NodeOutdatedTaint = "tuppr.home-operations.com/outdated"
)

// Talos config secret constants
const (
	TalosSecretName = "talosconfig"
	TalosSecretKey  = "config"
)

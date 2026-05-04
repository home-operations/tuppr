package constants

// Default image constants
const (
	DefaultTalosctlImage = "ghcr.io/siderolabs/talosctl"
)

// Annotation keys
const (
	ResetAnnotation   = "tuppr.home-operations.com/reset"
	SuspendAnnotation = "tuppr.home-operations.com/suspend"
)

const (
	// Override annotations
	SchematicAnnotation  = "tuppr.home-operations.com/schematic"
	VersionAnnotation    = "tuppr.home-operations.com/version"
	FactoryURLAnnotation = "tuppr.home-operations.com/factory-url"

	// Used as a fallback when SchematicAnnotation is not set.
	TalosSchematicAnnotation = "extensions.talos.dev/schematic"

	FactoryDomain = "factory.talos.dev"
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

package constants

// Default image constants
const (
	DefaultTalosctlImage = "ghcr.io/siderolabs/talosctl"
	GenericInstallerRepo = "ghcr.io/siderolabs/installer"
	// DefaultSchematic is the Image Factory's empty schematic, the installer
	// Talos itself defaults to. It carries no extensions, so it is what the
	// generic installer stood for before Talos 1.14 stopped publishing it.
	DefaultSchematic = "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba"
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

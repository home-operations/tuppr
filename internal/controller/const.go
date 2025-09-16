package controller

const (
	// Shared constants with TalosReconciler
	TalosConfigSecretName = "talup"
	TalosConfigSecretKey  = "talosconfig"

	// Phase constants
	PhasePending    = "Pending"
	PhaseInProgress = "InProgress"
	PhaseCompleted  = "Completed"
	PhaseFailed     = "Failed"
)

package controller

const (
	// Shared constants with TalosReconciler
	TalosConfigSecretName = "talup"
	TalosConfigSecretKey  = "config"

	// Phase constants
	PhasePending    = "Pending"
	PhaseInProgress = "InProgress"
	PhaseCompleted  = "Completed"
	PhaseFailed     = "Failed"
)

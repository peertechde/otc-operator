package provider

type State int

const (
	// Unknown indicates the state cannot be determined.
	Unknown State = iota
	// Provisioning indicates the resource is being created, updated or is in
	// another transient state.
	Provisioning
	// Ready indicates the resource is active, available and fully functional.
	Ready
	// Stopped indicates the resource is in a valid, but inactive state (e.g.,
	// powered off).
	Stopped
	// Failed indicates the resource is in an error or failed state.
	Failed
)

type Checker interface {
	// State returns the generalized state of the resource.
	State() State
	// Message returns the status message from the provider for logging and
	// conditions.
	Message() string
}

func IsProvisioning(s Checker) bool {
	return s.State() == Provisioning
}

func IsReady(s Checker) bool {
	return s.State() == Ready
}

func IsStopped(s Checker) bool {
	return s.State() == Stopped
}

func IsFailed(s Checker) bool {
	return s.State() == Failed
}

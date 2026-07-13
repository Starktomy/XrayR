// Package service contains all the services used by XrayR
// To implement a service, one needs to implement the interface below.
package service

// Service is the interface of all the services running in the panel.
// A Service can be started and closed; Restart is a no-op alias kept
// only for backward compatibility with embedders that implemented the
// older two-method interface.
type Service interface {
	Start() error
	Close() error
}

// Restart is a deprecated alias for the Start/Close pair. Existing
// implementations can keep it satisfied without changing their code;
// new code should embed Service directly.
type Restart interface {
	Start() error
	Close() error
}

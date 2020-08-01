package reload

import (
	"context"
	"sync"
)

// Component is a part of a system that can be reloaded.
type Component interface {
	// LockForReload locks the Component such that it is safe to mutate its underlying dependencies or configuration.
	LockForReload(ctx context.Context) error

	// UnlockAndReload unlocks the component and reloads its underlying dependencies or configuration.
	UnlockAndReload(ctx context.Context) error
}

// System is a orchestration unit for components that must all be reloaded together.
type System struct {
	mutex      sync.Mutex
	components []Component
}

// Register registers a component into this reload System.
func (s *System) Register(c Component) {
	s.mutex.Lock()
	s.components = append(s.components, c)
	s.mutex.Unlock()
}

// Reload locks all components, calls f, and unlocks all components.
func (s *System) Reload(ctx context.Context, f func() error) (_err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	var lockedComponents []Component
	defer func() {
		for len(lockedComponents) > 0 {
			c := lockedComponents[len(lockedComponents)-1]
			lockedComponents = lockedComponents[:len(lockedComponents)-1] // AKA pop
			err := c.UnlockAndReload(ctx)
			if err != nil && _err == nil {
				_err = err
			}
		}
	}()

	for _, c := range s.components {
		err := c.LockForReload(ctx)
		if err != nil {
			return err
		}
		lockedComponents = append(lockedComponents, c)
	}

	return f()
}

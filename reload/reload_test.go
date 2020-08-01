package reload_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/hannibal/reload"
	"github.com/stretchr/testify/assert"
)

func TestSystemReloadEmpty(t *testing.T) {
	s := &reload.System{}
	err := s.Reload(context.Background(), func() error { return nil })
	assert.NoError(t, err)
}

type testComponent struct {
	name                 string
	callLog              *[]string
	lockForReloadError   error
	unlockAndReloadError error
}

func (c *testComponent) LockForReload(ctx context.Context) error {
	*c.callLog = append(*c.callLog, fmt.Sprintf("%s lock", c.name))
	return c.lockForReloadError
}

// UnlockAndReload unlocks the component and reloads its underlying dependencies or configuration.
func (c *testComponent) UnlockAndReload(ctx context.Context) error {
	*c.callLog = append(*c.callLog, fmt.Sprintf("%s unlock", c.name))
	return c.unlockAndReloadError
}

func TestSystemReload(t *testing.T) {
	var callLog []string

	c1 := &testComponent{"c1", &callLog, nil, nil}
	c2 := &testComponent{"c2", &callLog, nil, nil}

	s := &reload.System{}
	s.Register(c1)
	s.Register(c2)

	err := s.Reload(context.Background(), func() error { return nil })
	assert.NoError(t, err)

	assert.Equal(t, []string{"c1 lock", "c2 lock", "c2 unlock", "c1 unlock"}, callLog)
}

func TestSystemFuncFailureUnlocksComponents(t *testing.T) {
	var callLog []string

	c1 := &testComponent{"c1", &callLog, nil, nil}
	c2 := &testComponent{"c2", &callLog, nil, nil}

	s := &reload.System{}
	s.Register(c1)
	s.Register(c2)

	err := s.Reload(context.Background(), func() error { return errors.New("foo") })
	assert.EqualError(t, err, "foo")

	assert.Equal(t, []string{"c1 lock", "c2 lock", "c2 unlock", "c1 unlock"}, callLog)
}

func TestSystemLockFailureUnlocksLockedComponents(t *testing.T) {
	var callLog []string

	c1 := &testComponent{"c1", &callLog, nil, nil}
	c2 := &testComponent{"c2", &callLog, errors.New("foo"), nil}

	s := &reload.System{}
	s.Register(c1)
	s.Register(c2)

	err := s.Reload(context.Background(), func() error { return nil })
	assert.EqualError(t, err, "foo")

	assert.Equal(t, []string{"c1 lock", "c2 lock", "c1 unlock"}, callLog)
}

func TestSystemUnlockFailureUnlocksLockedComponents(t *testing.T) {
	var callLog []string

	c1 := &testComponent{"c1", &callLog, nil, nil}
	c2 := &testComponent{"c2", &callLog, nil, errors.New("foo")}

	s := &reload.System{}
	s.Register(c1)
	s.Register(c2)

	err := s.Reload(context.Background(), func() error { return nil })
	assert.EqualError(t, err, "foo")

	assert.Equal(t, []string{"c1 lock", "c2 lock", "c2 unlock", "c1 unlock"}, callLog)
}

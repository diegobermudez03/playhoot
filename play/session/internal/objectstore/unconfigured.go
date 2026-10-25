package objectstore

import (
	"context"
	"errors"
	"time"
)

// ErrNotConfigured is returned by every call to an Unconfigured store.
var ErrNotConfigured = errors.New("object storage is not configured")

// Unconfigured is the Store of an environment without object storage: every
// call fails with ErrNotConfigured.
type Unconfigured struct{}

// Get implements Store.
func (Unconfigured) Get(context.Context, string) ([]byte, error) {
	return nil, ErrNotConfigured
}

// Put implements Store.
func (Unconfigured) Put(context.Context, string, []byte, string) error {
	return ErrNotConfigured
}

// PresignGet implements Store.
func (Unconfigured) PresignGet(context.Context, string, time.Time) (string, error) {
	return "", ErrNotConfigured
}

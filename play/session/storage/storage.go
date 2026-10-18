// Package storage opens the private object storage where the Session domain
// keeps game version content (backend scripts and frontend packages).
package storage

import (
	"context"
	"errors"

	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
)

// Store is the private object storage game version content lives in.
type Store = objectstore.Store

// Open returns the Google Cloud Storage bucket of projectID, and the function
// that releases it. Both empty means no storage is configured: every call then
// fails. The client also honors STORAGE_EMULATOR_HOST and the standard Google
// credentials variables, so an emulator needs no code change.
func Open(ctx context.Context, projectID string, bucket string) (Store, func() error, error) {
	if projectID == "" && bucket == "" {
		return objectstore.Unconfigured{}, func() error { return nil }, nil
	}
	if projectID == "" || bucket == "" {
		return nil, nil, errors.New("object storage needs both a project id and a bucket")
	}
	gcs, err := objectstore.NewGCS(ctx, projectID, bucket)
	if err != nil {
		return nil, nil, err
	}
	return gcs, gcs.Close, nil
}

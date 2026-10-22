package liveconnection

import (
	"context"

	"github.com/diegobermudez03/playhoot/play/runtime/usecases/limitaudit"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// limitHitStore records limit hits for auditing.
type limitHitStore interface {
	RecordLimitHits(ctx context.Context, hits []sessionpkg.LimitHit) (int, error)
}

// limitHitAuditor gathers the limits a Session's users hit for its audit
// records; Hit never waits for storage.
type limitHitAuditor interface {
	Hit(ctx context.Context, sessionUUID, userUUID, limit string)
}

// auditor is where limit hits go; without one they are discarded.
func (d *controllerDependencies) auditor() limitHitAuditor {
	if d.limitHits == nil {
		return noLimitAudit{}
	}
	return d.limitHits
}

// noLimitAudit discards limit hits, for a resolver built without storage.
type noLimitAudit struct{}

func (noLimitAudit) Hit(context.Context, string, string, string) {}

// startLimitAudit returns the auditor for limit hits stored through store,
// recording them in the background until root is done; without a store it
// discards them.
func (r *ControllerResolver) startLimitAudit(store limitHitStore) limitHitAuditor {
	if store == nil {
		return noLimitAudit{}
	}
	auditor := limitaudit.New(store)
	r.running.Add(1)
	go func() {
		defer r.running.Done()
		auditor.Run(r.root)
	}()
	return auditor
}

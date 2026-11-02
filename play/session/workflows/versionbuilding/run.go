package versionbuilding

import (
	"context"
	"sync"
	"time"

	"github.com/diegobermudez03/playhoot/logging"
)

// Run requeues the builds an earlier run left running, then runs the
// instance's workers until ctx is done. Each worker runs queued builds until
// none is left, then waits to be woken or for its periodic check.
func (m *Manager) Run(ctx context.Context) {
	startCtx := logging.Start(ctx)
	if _, err := m.RequeueInterrupted(startCtx); err != nil {
		logging.LogError(startCtx, err)
	}
	logging.FinishRequestLog(startCtx, m.logger, "build worker start")

	var workers sync.WaitGroup
	for range m.workers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			m.work(ctx)
		}()
	}
	workers.Wait()
}

func (m *Manager) work(ctx context.Context) {
	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()
	for {
		for m.runOne(ctx) {
		}
		select {
		case <-ctx.Done():
			return
		case <-m.signal.C():
		case <-ticker.C:
		}
	}
}

// runOne runs one queued build as its own logged entry point and reports
// whether to look for another right away.
func (m *Manager) runOne(ctx context.Context) bool {
	if ctx.Err() != nil {
		return false
	}
	buildCtx := logging.Start(ctx)
	ran, err := m.RunNextBuild(buildCtx)
	if err != nil {
		logging.LogError(buildCtx, err)
	}
	if ran || err != nil {
		logging.FinishRequestLog(buildCtx, m.logger, "draft build")
	}
	return ran && err == nil
}

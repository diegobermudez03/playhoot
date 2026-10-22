package liveconnection

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/utils/ratelimit"
)

// auditedHits is a limitHitAuditor keeping every hit.
type auditedHits struct {
	mu   sync.Mutex
	hits []string
}

func (a *auditedHits) Hit(_ context.Context, sessionUUID, userUUID, limit string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.hits = append(a.hits, sessionUUID+" "+userUUID+" "+limit)
}

func (a *auditedHits) recorded() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.hits...)
}

func TestRuntimeRefusalsAreAuditedForTheirUser(t *testing.T) {
	ctx := context.Background()
	controller, players, _ := runningControllerWithTokens(t, map[string][]byte{"P1": []byte(`{}`)}, nil, func(limits *liveLimits) {
		limits.participantCommands = []ratelimit.Limit{{Count: 1, Per: time.Second}}
	}, "user-a")
	audit := &auditedHits{}
	controller.deps.limitHits = audit
	sendCommand(players["user-a"], runtime.Command{Token: "t1", Name: "pass"})
	waitForTimeline(t, controller, 1)

	result, err := players["user-a"].Command(ctx, runtime.Command{Token: "t2", Name: "pass"})

	require.NoError(t, err)
	require.Equal(t, runtime.CommandRateLimited, result.Outcome)
	require.Equal(t, []string{"session-1 user-a runtime.command_participant_rate"}, audit.recorded())
}

func TestEntryPointLimitsAreAuditedForTheirUser(t *testing.T) {
	ctx := context.Background()
	controller, players, _ := runningController(t, map[string][]byte{"P1": []byte(`{}`)}, "user-a")
	audit := &auditedHits{}
	controller.deps.limitHits = audit

	players["user-a"].LimitHit(ctx, "api.command.rate")

	require.Equal(t, []string{"session-1 user-a api.command.rate"}, audit.recorded())
}

func TestWithoutAnAuditorLimitHitsAreDiscarded(t *testing.T) {
	controller, players, _ := runningController(t, map[string][]byte{"P1": []byte(`{}`)}, "user-a")
	controller.deps.limitHits = nil

	players["user-a"].LimitHit(context.Background(), "api.command.rate")
}

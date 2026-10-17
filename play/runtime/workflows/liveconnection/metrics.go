package liveconnection

import (
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

var (
	attachOutcomeLabel = metrics.NewLabel("outcome",
		"accepted",
		string(runtime.RejectNotCreator),
		string(runtime.RejectSessionUnavailable),
		string(runtime.RejectOwnerChanged),
	)

	adminAttaches = metrics.NewCounter(
		"play_runtime_admin_attach_total",
		"Admin connections attached to a live Session, by outcome.",
		attachOutcomeLabel,
	)
)

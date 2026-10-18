package controller

import "github.com/diegobermudez03/playhoot/monitoring/metrics"

var (
	controllersCreated = metrics.NewCounter(
		"play_runtime_controllers_created_total",
		"Session controllers this instance created.",
	)
	controllersRetired = metrics.NewCounter(
		"play_runtime_controllers_retired_total",
		"Session controllers replaced because their ownership epoch was stale.",
	)
)

var playersReplaced = metrics.NewCounter(
	"play_runtime_player_connections_replaced_total",
	"Player connections ended because the same user opened a newer one.",
)

package monitoring

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/logging"
)

// Alert raises an operational alert for message. It logs message under the
// request log's alert_msg field first, so the log entry that triggered the
// alert can be found by searching for that exact message.
func Alert(ctx context.Context, message string) {
	logging.LogFields(ctx, logging.Field("alert_msg", message))
	// TODO: forward message to a real alerting system once one is integrated.
	fmt.Println(message)
}

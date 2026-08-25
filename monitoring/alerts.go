package monitoring

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/logging"
)

// Alert triggers an alert with the passed message and will attach the alert message into the log
func Alert(ctx context.Context, message string) {
	// to guarantee that the log will contain the same message, then when we receive the alert we can look for the log that has the alert message
	logging.LogFields(ctx, logging.Field("alert_msg", message))
	// here we should eventually trigger an actual alert with the whatever system we have integrated
	fmt.Println(message)
}

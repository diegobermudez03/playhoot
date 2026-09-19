package sessionlifecycle

import (
	"encoding/json"
	"fmt"
)

// Session lifecycle idempotency operation labels
// (`docs/engineering/standards/idempotency.md`).
const (
	operationCreate = "CREATE"
	operationJoin   = "JOIN"
	operationLeave  = "LEAVE"
)

// marshalPayload is the only genuinely mechanical, non-interpretive helper
// shared across steps' idempotency payloads
// (`docs/engineering/standards/idempotency.md`'s Organizational Guidance).
// Each step's own payload struct and interpretation function live next to
// that step instead (step_create.go/step_join.go/step_leave.go).
func marshalPayload(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshaling idempotency request payload: %s", err)
	}
	return string(b), nil
}

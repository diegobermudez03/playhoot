package txqueue

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// taskRow is a queue table row as Enqueue inserts it.
type taskRow struct {
	UUID           string    `gorm:"column:uuid"`
	Type           string    `gorm:"column:type"`
	Payload        string    `gorm:"column:payload;type:jsonb"`
	IdempotencyKey *string   `gorm:"column:idempotency_key"`
	Status         string    `gorm:"column:status"`
	RunAt          time.Time `gorm:"column:run_at"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

// claimedRow is a task the worker took.
type claimedRow struct {
	UUID     string `gorm:"column:uuid"`
	Type     string `gorm:"column:type"`
	Payload  string `gorm:"column:payload"`
	Attempts int    `gorm:"column:attempts"`
}

// deadRow is a dead task as admin lists it.
type deadRow struct {
	UUID         string    `gorm:"column:uuid"`
	Type         string    `gorm:"column:type"`
	Payload      string    `gorm:"column:payload"`
	Attempts     int       `gorm:"column:attempts"`
	LastError    *string   `gorm:"column:last_error"`
	AdminRetries int       `gorm:"column:admin_retries"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	DiedAt       time.Time `gorm:"column:run_at"`
}

// store is the SQL over one queue table. The table name comes from a checked
// domain name; every value is bound.
type store struct {
	db    *gorm.DB
	table string
}

func (s *store) GetDB() *gorm.DB {
	return s.db
}

// claim takes up to limit due tasks of types: pending ones whose run_at came,
// and running ones whose lease ran out (their worker died). A claimed task is
// running with run_at at the end of its lease.
func (s *store) claim(ctx context.Context, types []string, now time.Time, leaseUntil time.Time, limit int) ([]claimedRow, error) {
	var rows []claimedRow
	query := fmt.Sprintf(`
		UPDATE %[1]s SET status = ?, attempts = attempts + 1, run_at = ?, updated_at = ?
		WHERE id IN (
			SELECT id FROM %[1]s
			WHERE status IN (?, ?) AND run_at <= ? AND type IN ?
			ORDER BY run_at
			LIMIT ?
			FOR UPDATE SKIP LOCKED
		)
		RETURNING uuid, type, payload, attempts`, s.table)
	err := s.db.WithContext(ctx).Raw(query, StatusRunning, leaseUntil, now, StatusPending, StatusRunning, now, types, limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("claiming tasks from %s: %w", s.table, err)
	}
	return rows, nil
}

// complete deletes a task the worker still holds (same attempt), reporting
// whether it did.
func (s *store) complete(ctx context.Context, tx *gorm.DB, taskUUID string, attempt int) (bool, error) {
	result := tx.WithContext(ctx).Exec(fmt.Sprintf(`DELETE FROM %s WHERE uuid = ? AND status = ? AND attempts = ?`, s.table), taskUUID, StatusRunning, attempt)
	if result.Error != nil {
		return false, fmt.Errorf("completing task on %s: %w", s.table, result.Error)
	}
	return result.RowsAffected == 1, nil
}

// fail moves a task the worker still holds to status (pending for a retry at
// runAt, dead for good), recording why.
func (s *store) fail(ctx context.Context, taskUUID string, attempt int, status string, runAt time.Time, lastError string, now time.Time) error {
	err := s.db.WithContext(ctx).Exec(fmt.Sprintf(`UPDATE %s SET status = ?, run_at = ?, last_error = ?, updated_at = ? WHERE uuid = ? AND status = ? AND attempts = ?`, s.table),
		status, runAt, lastError, now, taskUUID, StatusRunning, attempt).Error
	if err != nil {
		return fmt.Errorf("recording failed task on %s: %w", s.table, err)
	}
	return nil
}

// listDead lists up to limit dead tasks, the most recently dead first.
func (s *store) listDead(ctx context.Context, limit int) ([]deadRow, error) {
	var rows []deadRow
	err := s.db.WithContext(ctx).Raw(fmt.Sprintf(`SELECT uuid, type, payload, attempts, last_error, admin_retries, created_at, run_at FROM %s WHERE status = ? ORDER BY run_at DESC LIMIT ?`, s.table),
		StatusDead, limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("listing dead tasks of %s: %w", s.table, err)
	}
	return rows, nil
}

// retryDead puts a dead task back to pending with fresh attempts, due now.
func (s *store) retryDead(ctx context.Context, taskUUID string, now time.Time) (bool, error) {
	result := s.db.WithContext(ctx).Exec(fmt.Sprintf(`UPDATE %s SET status = ?, attempts = 0, run_at = ?, admin_retries = admin_retries + 1, updated_at = ? WHERE uuid = ? AND status = ?`, s.table),
		StatusPending, now, now, taskUUID, StatusDead)
	if result.Error != nil {
		return false, fmt.Errorf("retrying dead task on %s: %w", s.table, result.Error)
	}
	return result.RowsAffected == 1, nil
}

// cancelDead cancels a dead task; it is kept, never run again.
func (s *store) cancelDead(ctx context.Context, taskUUID string, now time.Time) (bool, error) {
	result := s.db.WithContext(ctx).Exec(fmt.Sprintf(`UPDATE %s SET status = ?, updated_at = ? WHERE uuid = ? AND status = ?`, s.table),
		StatusCancelled, now, taskUUID, StatusDead)
	if result.Error != nil {
		return false, fmt.Errorf("cancelling dead task on %s: %w", s.table, result.Error)
	}
	return result.RowsAffected == 1, nil
}

// has reports whether the queue holds the task, in any status.
func (s *store) has(ctx context.Context, taskUUID string) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Raw(fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE uuid = ?`, s.table), taskUUID).Scan(&count).Error
	if err != nil {
		return false, fmt.Errorf("finding task on %s: %w", s.table, err)
	}
	return count > 0, nil
}

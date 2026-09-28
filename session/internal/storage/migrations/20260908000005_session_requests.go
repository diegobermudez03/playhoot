package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration20260908000005SessionRequests() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260908000005_session_requests",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				CREATE TABLE session_requests (
					id BIGSERIAL PRIMARY KEY,
					operation TEXT NOT NULL,
					idempotency_key TEXT NOT NULL,
					user_uuid UUID NOT NULL,
					session_id BIGINT NULL,
					request_payload JSONB NOT NULL,
					outcome TEXT NOT NULL DEFAULT '',
					response_payload JSONB NULL,
					status TEXT NOT NULL DEFAULT 'PENDING',
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					CONSTRAINT session_requests_user_uuid_operation_idempotency_key_key
						UNIQUE (user_uuid, operation, idempotency_key)
				)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE session_requests`).Error
		},
	}
}

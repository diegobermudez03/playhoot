package migrations

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/compliance"
)

type acceptanceTextRow struct {
	ID          int64
	Kind        string
	Version     int
	Language    string
	Content     string
	PublishedAt time.Time
	CreatedAt   time.Time
}

func (acceptanceTextRow) TableName() string { return "compliance_acceptance_texts" }

var draftTitles = map[compliance.TextKind]string{
	compliance.TextTerms:              "Terms of use and purchase",
	compliance.TextOwnData:            "Processing of your own data",
	compliance.TextClassDataAgreement: "Class data processing agreement",
	compliance.TextMarketing:          "Marketing communications",
}

// SeedDraftTexts publishes version 1 of every kind and language as a draft
// placeholder, skipping any kind and language that already has a text.
func SeedDraftTexts(db *gorm.DB) error {
	now := time.Now()
	for _, kind := range compliance.TextKinds {
		for _, language := range compliance.Languages {
			var count int64
			err := db.Raw(`SELECT COUNT(*) FROM compliance_acceptance_texts WHERE kind = ? AND language = ?`, string(kind), language).Scan(&count).Error
			if err != nil {
				return fmt.Errorf("reading acceptance texts: %w", err)
			}
			if count > 0 {
				continue
			}
			row := acceptanceTextRow{
				Kind: string(kind), Version: 1, Language: language, PublishedAt: now, CreatedAt: now,
				Content: fmt.Sprintf("DRAFT PLACEHOLDER (%s) — %s. Not the real text: replace before any market opens.", language, draftTitles[kind]),
			}
			if err := db.Create(&row).Error; err != nil {
				return fmt.Errorf("seeding acceptance text: %w", err)
			}
		}
	}
	return nil
}

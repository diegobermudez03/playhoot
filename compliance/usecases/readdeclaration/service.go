// Package readdeclaration answers what a person declared about where they
// live and the age that place asks for. It is what they declared, never
// something Playhoot verified.
package readdeclaration

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/compliance"
	internalrepo "github.com/diegobermudez03/playhoot/compliance/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
)

// Service reads a person's current declaration.
type Service struct {
	repo *internalrepo.Repo
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{repo: internalrepo.New(db)}
}

// ReadDeclaration returns the declaration in force for userUUID, or none.
// Corrections are not made here: they go through support.
func (s *Service) ReadDeclaration(ctx context.Context, userUUID string) (compliance.CurrentDeclaration, error) {
	defer logging.Step(ctx, "compliance.read_declaration").Close()
	if _, err := uuid.Parse(userUUID); err != nil {
		return compliance.CurrentDeclaration{}, fmt.Errorf("invalid user uuid: %w", err)
	}
	declaration, err := s.repo.LatestDeclaration(ctx, s.repo.GetDB(), userUUID)
	if err != nil || declaration == nil {
		return compliance.CurrentDeclaration{}, err
	}
	return compliance.CurrentDeclaration{
		Found:        true,
		Country:      declaration.Country,
		Subdivision:  declaration.Subdivision,
		MeetsAge:     declaration.MeetsAge,
		ThresholdAge: declaration.ThresholdAge,
		DeclaredAt:   declaration.CreatedAt,
	}, nil
}

// Package mydata composes the one read that tells a client who the caller
// is: their identity and mode, the language they chose, the ways into their
// account, what they declared, and the capacities they have. A piece that
// cannot be read is answered as missing, never as a failure: the profile
// shows it as unknown.
package mydata

import (
	"context"

	"github.com/diegobermudez03/playhoot/compliance"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/plan"
)

//go:generate mockgen -package=mydata -destination=manager_mock_test.go . profileReader,declarationReader,limitReader

// profileReader is Identity's answer to who the caller is.
type profileReader interface {
	ReadProfile(ctx context.Context, userUUID string) (identity.Profile, error)
}

// declarationReader is Compliance's answer to what they declared.
type declarationReader interface {
	ReadDeclaration(ctx context.Context, userUUID string) (compliance.CurrentDeclaration, error)
}

// limitReader is Plan's answer to what they are entitled to.
type limitReader interface {
	GetUserLimits(ctx context.Context, userUUID string) (plan.Limits, error)
}

// MyData is everything the caller's own screens need about themselves.
// DeclarationKnown is false only when Compliance could not be asked; for a
// guest it is true and the declaration is simply not there. LimitsKnown is
// false when Plan could not be asked.
type MyData struct {
	Profile          identity.Profile
	Declaration      compliance.CurrentDeclaration
	DeclarationKnown bool
	Limits           plan.Limits
	LimitsKnown      bool
}

// Manager composes the caller's own data. It owns no business entity.
type Manager struct {
	profiles     profileReader
	declarations declarationReader
	limits       limitReader
}

// New constructs a Manager from Identity's profile, Compliance's
// declaration and Plan's capacities.
func New(profiles profileReader, declarations declarationReader, limits limitReader) *Manager {
	return &Manager{profiles: profiles, declarations: declarations, limits: limits}
}

// ReadMyData returns who userUUID is, with whatever of the declaration and
// the capacities could be read. identity.ErrUserNotFound when the caller has
// no identity: without one there is nothing to answer.
func (m *Manager) ReadMyData(ctx context.Context, userUUID string) (MyData, error) {
	defer logging.Step(ctx, "composer.read_my_data").Close()

	profile, err := m.profiles.ReadProfile(ctx, userUUID)
	if err != nil {
		return MyData{}, err
	}
	data := MyData{Profile: profile, DeclarationKnown: true}
	if profile.Level == identity.LevelAccount {
		declaration, err := m.declarations.ReadDeclaration(ctx, userUUID)
		if err != nil {
			logging.LogError(ctx, err)
			data.DeclarationKnown = false
		} else {
			data.Declaration = declaration
		}
	}
	limits, err := m.limits.GetUserLimits(ctx, userUUID)
	if err != nil {
		logging.LogError(ctx, err)
		return data, nil
	}
	data.Limits, data.LimitsKnown = limits, true
	return data, nil
}

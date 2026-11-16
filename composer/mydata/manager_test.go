package mydata

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/compliance"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/plan"
)

const testUserUUID = "0f8fad5b-d9cb-469f-a165-70867728950e"

func TestReadMyData(t *testing.T) {
	type test struct {
		expected  MyData
		expectErr error
	}

	ctx := context.Background()
	declaredAt := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	limits := plan.Limits{SavedGames: 5, CreationsPerWindow: 3, SessionsPerWindow: 10, WindowDays: 30, PlayersPerSession: 30, SimultaneousRooms: 1, StorageMB: 100}
	account := identity.Profile{UUID: testUserUUID, DisplayName: "Ana", Level: identity.LevelAccount, Mode: identity.ModeTeacher, Language: "es"}
	guest := identity.Profile{UUID: testUserUUID, DisplayName: "Ana", Level: identity.LevelGuest, Mode: identity.ModeNormal}
	declaration := compliance.CurrentDeclaration{Found: true, Country: "CO", MeetsAge: true, ThresholdAge: 18, DeclaredAt: declaredAt}

	type dependencies struct {
		profiles     *MockprofileReader
		declarations *MockdeclarationReader
		limits       *MocklimitReader
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"an account carries its declaration and capacities": func(t *testing.T, deps dependencies) test {
			deps.profiles.EXPECT().ReadProfile(ctx, testUserUUID).Return(account, nil)
			deps.declarations.EXPECT().ReadDeclaration(ctx, testUserUUID).Return(declaration, nil)
			deps.limits.EXPECT().GetUserLimits(ctx, testUserUUID).Return(limits, nil)
			return test{expected: MyData{
				Profile: account, Declaration: declaration, DeclarationKnown: true, Limits: limits, LimitsKnown: true,
			}}
		},
		"a guest is never asked for a declaration and still gets its capacities": func(t *testing.T, deps dependencies) test {
			deps.profiles.EXPECT().ReadProfile(ctx, testUserUUID).Return(guest, nil)
			deps.limits.EXPECT().GetUserLimits(ctx, testUserUUID).Return(limits, nil)
			return test{expected: MyData{Profile: guest, DeclarationKnown: true, Limits: limits, LimitsKnown: true}}
		},
		"a declaration that cannot be read is answered as unknown": func(t *testing.T, deps dependencies) test {
			deps.profiles.EXPECT().ReadProfile(ctx, testUserUUID).Return(account, nil)
			deps.declarations.EXPECT().ReadDeclaration(ctx, testUserUUID).Return(compliance.CurrentDeclaration{}, errors.New("down"))
			deps.limits.EXPECT().GetUserLimits(ctx, testUserUUID).Return(limits, nil)
			return test{expected: MyData{Profile: account, Limits: limits, LimitsKnown: true}}
		},
		"capacities that cannot be read are answered as unknown": func(t *testing.T, deps dependencies) test {
			deps.profiles.EXPECT().ReadProfile(ctx, testUserUUID).Return(account, nil)
			deps.declarations.EXPECT().ReadDeclaration(ctx, testUserUUID).Return(declaration, nil)
			deps.limits.EXPECT().GetUserLimits(ctx, testUserUUID).Return(plan.Limits{}, errors.New("down"))
			return test{expected: MyData{Profile: account, Declaration: declaration, DeclarationKnown: true}}
		},
		"without an identity there is nothing to answer": func(t *testing.T, deps dependencies) test {
			deps.profiles.EXPECT().ReadProfile(ctx, testUserUUID).Return(identity.Profile{}, identity.ErrUserNotFound)
			return test{expectErr: identity.ErrUserNotFound}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{profiles: NewMockprofileReader(ctrl), declarations: NewMockdeclarationReader(ctrl), limits: NewMocklimitReader(ctrl)}
			tc := setup(t, deps)
			manager := New(deps.profiles, deps.declarations, deps.limits)

			got, err := manager.ReadMyData(ctx, testUserUUID)

			if tc.expectErr != nil {
				require.ErrorIs(t, err, tc.expectErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, got)
		})
	}
}

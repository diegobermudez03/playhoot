package sessionlifecycle

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestManagerMapOutputs covers mapOutputs' translation from engine's Output/
// Value schema into sessionlifecycle's own exported schema - the only part
// of this file testable without a real Postgres transaction, since the
// repository lookup is mocked. Recipient/nested-UserValue resolution against
// a real session_actors table is covered by
// TestRepoFindActorsByIDs/the *_Integration tests exercising Start/
// AnswerInteraction end-to-end.
func TestManagerMapOutputs(t *testing.T) {
	t.Run("maps every Value variant, resolving nested and top-level recipients", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := NewMockoutputsRepoAPI(ctrl)

		hostActorID, playerActorID := uint(1), uint(2)
		hostUUID, playerUUID := "host-uuid", "player-uuid"
		repo.EXPECT().
			FindActorsByIDs(gomock.Any(), gomock.Any(), uint(7), gomock.InAnyOrder([]uint{hostActorID, playerActorID})).
			Return([]internalrepo.Actor{
				{ID: hostActorID, SessionID: 7, UserUUID: hostUUID},
				{ID: playerActorID, SessionID: 7, UserUUID: playerUUID},
			}, nil)

		m := &Manager{outputsRepo: repo}

		hostID := engine.UserID(strconv.FormatUint(uint64(hostActorID), 10))
		playerID := engine.UserID(strconv.FormatUint(uint64(playerActorID), 10))

		// A record nesting one instance of every remaining Value kind,
		// exercised as one Presentation's Model.
		model := engine.RecordValue{
			TypeName: "Hud",
			Fields: []engine.FieldValue{
				{Name: "unit", Value: engine.UnitValue{}},
				{Name: "ready", Value: engine.BoolValue{Value: true}},
				{Name: "score", Value: engine.NumberValue{Value: 42}},
				{Name: "label", Value: engine.StringValue{Value: "hi"}},
				{Name: "turn_player", Value: engine.UserValue{ID: playerID}},
				{Name: "color", Value: engine.EnumValue{TypeName: "Color", ValueName: "RED"}},
				{Name: "outcome", Value: engine.UnionValue{
					TypeName:    "Outcome",
					VariantName: "Won",
					Fields:      []engine.FieldValue{{Name: "by", Value: engine.NumberValue{Value: 3}}},
				}},
				{Name: "score_points", Value: engine.NewTypeValue{
					TypeName:   "Points",
					Underlying: engine.NumberValue{Value: 9},
				}},
				{Name: "bonus", Value: engine.OptionalValue{
					ElementType: engine.NumberType{},
					Value:       nil,
				}},
				{Name: "hand", Value: engine.ListValue{
					ElementType: engine.StringType{},
					Elements:    []engine.Value{engine.StringValue{Value: "card"}},
				}},
				{Name: "scores_by_player", Value: engine.MapValue{
					KeyType:   engine.UserType{},
					ValueType: engine.NumberType{},
					Entries: []engine.MapEntry{
						{Key: engine.UserValue{ID: playerID}, Value: engine.NumberValue{Value: 5}},
					},
				}},
			},
		}

		outputs := []engine.Output{
			engine.ActivatePresentationOutput{
				Slot:      "hud",
				Recipient: hostID,
				Name:      "Hud",
				View:      "HudView",
				Model:     model,
			},
		}

		mapped, err := m.mapOutputs(context.Background(), nil, 7, outputs)
		require.NoError(t, err)
		require.Len(t, mapped, 1)

		activated, ok := mapped[0].(PresentationActivated)
		require.True(t, ok, "%T", mapped[0])
		require.Equal(t, UserUUID(hostUUID), activated.Recipient)

		record, ok := activated.Model.(RecordValue)
		require.True(t, ok, "%T", activated.Model)
		fieldByName := make(map[string]Value, len(record.Fields))
		for _, f := range record.Fields {
			fieldByName[f.Name] = f.Value
		}

		require.Equal(t, UnitValue{}, fieldByName["unit"])
		require.Equal(t, BoolValue{Value: true}, fieldByName["ready"])
		require.Equal(t, NumberValue{Value: 42}, fieldByName["score"])
		require.Equal(t, StringValue{Value: "hi"}, fieldByName["label"])
		require.Equal(t, UserValue{UserUUID: UserUUID(playerUUID)}, fieldByName["turn_player"])
		require.Equal(t, EnumValue{TypeName: "Color", ValueName: "RED"}, fieldByName["color"])
		require.Equal(t, UnionValue{
			TypeName:    "Outcome",
			VariantName: "Won",
			Fields:      []FieldValue{{Name: "by", Value: NumberValue{Value: 3}}},
		}, fieldByName["outcome"])
		require.Equal(t, NewTypeValue{TypeName: "Points", Underlying: NumberValue{Value: 9}}, fieldByName["score_points"])
		require.Equal(t, OptionalValue{ElementType: NumberType{}}, fieldByName["bonus"])
		require.Equal(t, ListValue{ElementType: StringType{}, Elements: []Value{StringValue{Value: "card"}}}, fieldByName["hand"])
		require.Equal(t, MapValue{
			KeyType:   UserType{},
			ValueType: NumberType{},
			Entries:   []MapEntry{{Key: UserValue{UserUUID: UserUUID(playerUUID)}, Value: NumberValue{Value: 5}}},
		}, fieldByName["scores_by_player"])
	})

	t.Run("maps effect recipients and arguments", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := NewMockoutputsRepoAPI(ctrl)

		actorAUUID, actorBUUID := "a-uuid", "b-uuid"
		repo.EXPECT().
			FindActorsByIDs(gomock.Any(), gomock.Any(), uint(3), gomock.InAnyOrder([]uint{10, 20})).
			Return([]internalrepo.Actor{
				{ID: 10, SessionID: 3, UserUUID: actorAUUID},
				{ID: 20, SessionID: 3, UserUUID: actorBUUID},
			}, nil)

		m := &Manager{outputsRepo: repo}

		outputs := []engine.Output{
			engine.EmitEffectOutput{
				Effect: "Confetti",
				Recipients: []engine.UserID{
					engine.UserID(strconv.FormatUint(10, 10)),
					engine.UserID(strconv.FormatUint(20, 10)),
				},
				Arguments: []engine.FieldValue{{Name: "intensity", Value: engine.NumberValue{Value: 1}}},
			},
		}

		mapped, err := m.mapOutputs(context.Background(), nil, 3, outputs)
		require.NoError(t, err)
		require.Len(t, mapped, 1)

		emitted, ok := mapped[0].(EffectEmitted)
		require.True(t, ok, "%T", mapped[0])
		require.Equal(t, "Confetti", emitted.Effect)
		require.ElementsMatch(t, []UserUUID{UserUUID(actorAUUID), UserUUID(actorBUUID)}, emitted.Recipients)
		require.Equal(t, []FieldValue{{Name: "intensity", Value: NumberValue{Value: 1}}}, emitted.Arguments)
	})

	t.Run("returns nil for no outputs", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := NewMockoutputsRepoAPI(ctrl)
		repo.EXPECT().FindActorsByIDs(gomock.Any(), gomock.Any(), uint(1), []uint(nil)).Return(nil, nil)

		m := &Manager{outputsRepo: repo}
		mapped, err := m.mapOutputs(context.Background(), nil, 1, nil)
		require.NoError(t, err)
		require.Nil(t, mapped)
	})

	t.Run("fails when a recipient's actor id cannot be resolved", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := NewMockoutputsRepoAPI(ctrl)
		repo.EXPECT().FindActorsByIDs(gomock.Any(), gomock.Any(), uint(1), gomock.Any()).Return(nil, nil)

		m := &Manager{outputsRepo: repo}
		outputs := []engine.Output{
			engine.RemovePresentationOutput{Slot: "hud", Recipient: engine.UserID("99"), Name: "Hud"},
		}

		_, err := m.mapOutputs(context.Background(), nil, 1, outputs)
		require.Error(t, err)
	})

	t.Run("propagates a repository error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := NewMockoutputsRepoAPI(ctrl)
		repoErr := errors.New("repo failed")
		repo.EXPECT().FindActorsByIDs(gomock.Any(), gomock.Any(), uint(1), gomock.Any()).Return(nil, repoErr)

		m := &Manager{outputsRepo: repo}
		outputs := []engine.Output{
			engine.RemovePresentationOutput{Slot: "hud", Recipient: engine.UserID("1"), Name: "Hud"},
		}

		_, err := m.mapOutputs(context.Background(), nil, 1, outputs)
		require.ErrorContains(t, err, repoErr.Error())
	})
}

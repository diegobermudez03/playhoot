package game

import (
	"errors"
	"fmt"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
)

// This file sketches the boundary between a lobby/session layer and the
// compiled game engine.
//
// The program.Definition can declare that the root workflow needs players,
// for example a root parameter named "players" with type list<user>. It also
// declares its player-count policy, which the lobby reads from engine.Program
// instead of hardcoding. The application still owns auth, connections, host
// controls, and actually admitting users.

// lobby is application/session state, not engine state. A real version would
// live beside room membership, auth, websocket connections, host controls,
// persistence, and so on.
type lobby struct {
	program engine.Program
	users   []engine.UserID
}

func (l *lobby) Join(user engine.UserID) error {
	if user == "" {
		return errors.New("user id is required")
	}
	for _, existing := range l.users {
		if existing == user {
			return nil
		}
	}
	if l.program.Players.Max > 0 && len(l.users) >= l.program.Players.Max {
		return fmt.Errorf("room is full: max %d players", l.program.Players.Max)
	}
	l.users = append(l.users, user)
	return nil
}

func (l *lobby) Leave(user engine.UserID) {
	for i, existing := range l.users {
		if existing == user {
			l.users = append(l.users[:i], l.users[i+1:]...)
			return
		}
	}
}

func (l *lobby) CanStart() bool {
	if len(l.users) < l.program.Players.Min {
		return false
	}
	return l.program.Players.Max == 0 || len(l.users) <= l.program.Players.Max
}

// StartGame freezes the lobby membership into the root workflow parameters and
// creates the first engine Snapshot. The compiled Program must already have a
// root workflow parameter compatible with:
//
//	parameters: [{ name: "players", type: list<user> }]
//
// From this point on, users are ordinary engine values. The definition can
// store them in global state, use them as presentation targets, ask questions
// to them, check turn ownership, and bind the actor of user intents.
func (l *lobby) StartGame(seed uint64) (engine.Snapshot, engine.Signal, error) {
	if !l.CanStart() {
		max := "unbounded"
		if l.program.Players.Max > 0 {
			max = fmt.Sprintf("%d", l.program.Players.Max)
		}
		return engine.Snapshot{}, engine.Signal{}, fmt.Errorf(
			"cannot start game with %d players; need %d-%s",
			len(l.users),
			l.program.Players.Min,
			max,
		)
	}

	playerValues := make([]engine.Value, 0, len(l.users))
	for _, user := range l.users {
		playerValues = append(playerValues, engine.UserValue{ID: user})
	}

	return engineservice.NewSnapshot(l.program, engine.InitializationInput{
		RootParameters: map[string]engine.Value{
			"players": engine.ListValue{
				ElementType: engine.UserType{},
				Elements:    playerValues,
			},
		},
		Seed: seed,
	})
}

func exampleUsersSetup(compiledProgram engine.Program) error {
	l := lobby{program: compiledProgram}
	if err := l.Join(engine.UserID("alice")); err != nil {
		return err
	}
	if err := l.Join(engine.UserID("bob")); err != nil {
		return err
	}

	snapshot, startSignal, err := l.StartGame(12345)
	if err != nil {
		return err
	}

	commit, err := engineservice.Step(compiledProgram, snapshot, startSignal, engine.DefaultLimits())
	if err != nil {
		return err
	}

	_ = commit.Snapshot
	return nil
}

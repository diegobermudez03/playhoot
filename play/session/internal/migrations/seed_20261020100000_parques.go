package migrations

import (
	_ "embed"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
)

// Parqués test-data identifiers, so a person testing by hand can create its
// Sessions.
const (
	// seedParquesGameUUID is a playable Game whose EXECUTABLE version runs
	// seedParquesScript: the Colombian board game, built to exercise a
	// turn-based game played only through COMMANDs.
	seedParquesGameUUID    = "44444444-4444-4444-8444-444444444444"
	seedParquesVersionUUID = "44444444-4444-4444-8444-0000000000d1"

	// seedParquesManifest lets two to four players roll and move by command;
	// it takes no SAMPLE, update() runs out turn clocks, and a disconnected
	// player may rejoin.
	seedParquesManifest = `{"schema_version":1,"players":{"min":2,"max":4},"entrypoints":["update","playerRejoined"],` +
		`"commands":{"roll":{"payload":{}},` +
		`"move":{"payload":{"piece":{"type":"number","min":0,"max":3},"die":{"type":"enum","values":["a","b","sum"],"default":"sum"}}}}}`
)

// seedParquesScript is Parqués' backend script, test content rather than a
// real game; its opening comment explains the rules.
const seedParquesScript = `// Parqués: the Colombian race game for two to four players. Each player has
// four pieces that start in jail. The board is a ring of 68 squares, 17 per
// side, plus a lane of 7 squares from each side's tip to the centre (the
// cielo). On their turn a player rolls two dice (command roll) and moves with
// each die on a piece, or with their sum on one piece (command move {piece,
// die: a, b or sum}); the turn ends once the dice are used or no move fits.
// Pairs release every jailed piece onto the player's exit and roll again
// (pairs with nothing in jail move first, then roll again); the third pair in
// a row crowns the leading piece straight into the cielo and ends the turn.
// With no piece on the board, a player has three rolls to get pairs. Landing
// on another player's piece outside a safe square sends it to jail; pieces
// released onto an exit capture there even though it is safe. A piece enters
// the cielo by exact count, and the first player with all four there wins.
// A turn lasts 30 seconds: update() passes it when it runs out. A disconnected
// player's turns are skipped and they may rejoin. Refused commands answer
// {ok: false, reason} and change nothing. Every function returns new values
// instead of changing the ones it is given.
const RING = 68;
const SIDE = 17;
const ENTRY = 8;
const EXIT = 13;
const LAP = 63;
const GOAL = 71;
const JAIL = -1;
const PIECES = 4;
const MAX_JAIL_ROLLS = 3;
const TURN_MS = 30000;

function start(participants, context) {
	const seats = seatsFor(participants.length);
	const players = {};
	for (let index = 0; index < participants.length; index += 1) {
		players[participants[index]] = { seat: seats[index], pieces: [JAIL, JAIL, JAIL, JAIL], connected: true };
	}
	return {
		state: {
			order: participants.slice(),
			players: players,
			turn: newTurn(participants[0], context.sessionTime),
			lastRoll: null,
			winner: null,
		},
	};
}

function command(state, cmd, context) {
	if (state.winner !== null) {
		return refused(state, "GAME_OVER");
	}
	if (cmd.participant !== state.turn.player) {
		return refused(state, "NOT_YOUR_TURN");
	}
	if (cmd.name === "roll") {
		return roll(state, context.sessionTime);
	}
	return move(state, cmd.payload, context.sessionTime);
}

function update(state, context) {
	if (state.winner !== null || context.sessionTime < state.turn.deadline) {
		return { state: state };
	}
	const passed = nextTurn(state, context.sessionTime);
	return { state: passed.game, events: [{ event: { type: "TURN_TIMEOUT", player: state.turn.player } }].concat(passed.events) };
}

function playerDisconnected(state, participant, context) {
	const game = withPlayer(state, participant, merge(state.players[participant], { connected: false }));
	const events = [{ event: { type: "PLAYER_LEFT", player: participant } }];
	if (game.winner === null && game.turn.player === participant) {
		const passed = nextTurn(game, context.sessionTime);
		return { state: passed.game, events: events.concat(passed.events) };
	}
	return { state: game, events: events };
}

function playerRejoined(state, participant, context) {
	const game = withPlayer(state, participant, merge(state.players[participant], { connected: true }));
	return { state: game, accepted: true, events: [{ event: { type: "PLAYER_BACK", player: participant } }] };
}

function project(state, participant, context) {
	const myTurn = state.winner === null && state.turn.player === participant && state.turn.dice !== null;
	return {
		board: { ring: RING, side: SIDE, entry: ENTRY, exit: EXIT, safe: [3, ENTRY, EXIT], lap: LAP, goal: GOAL, turnMs: TURN_MS },
		players: state.order.map(function (id) {
			const player = state.players[id];
			return { id: id, seat: player.seat, pieces: player.pieces, connected: player.connected };
		}),
		turn: state.turn,
		lastRoll: state.lastRoll,
		winner: state.winner,
		moves: myTurn ? legalMoves(state, participant) : [],
	};
}

// roll throws the dice for the player whose turn it is.
function roll(state, now) {
	const turn = state.turn;
	if (turn.dice !== null) {
		return refused(state, "ALREADY_ROLLED");
	}
	const dice = [rollDie(), rollDie()];
	const pairs = dice[0] === dice[1];
	const id = turn.player;
	const player = state.players[id];
	const pairsInRow = pairs ? turn.pairs + 1 : turn.pairs;
	let game = merge(state, {
		lastRoll: { player: id, dice: dice },
		turn: merge(turn, { pairs: pairsInRow, deadline: now + TURN_MS }),
	});
	let events = [{ event: { type: "ROLLED", player: id, dice: dice } }];
	if (pairs && pairsInRow === 3) {
		const piece = leadingPiece(player);
		if (piece !== null) {
			game = withPiece(game, id, piece, GOAL);
			events = events.concat([{ event: { type: "CROWNED", player: id, piece: piece } }]);
			if (won(game.players[id])) {
				return finish(game, id, events, dice);
			}
		}
		const passed = nextTurn(game, now);
		return answer(passed.game, events.concat(passed.events), dice);
	}
	if (pairs && player.pieces.indexOf(JAIL) >= 0) {
		const released = player.pieces.map(function (progress) { return progress === JAIL ? 0 : progress; });
		const captured = capture(withPlayer(game, id, merge(player, { pieces: released })), id, square(player.seat, 0), true);
		game = merge(captured.game, { turn: merge(captured.game.turn, { jailRolls: 0 }) });
		return answer(game, events.concat([{ event: { type: "RELEASED", player: id } }], captured.events), dice);
	}
	if (leadingPiece(player) === null) {
		const jailRolls = turn.jailRolls + 1;
		game = merge(game, { turn: merge(game.turn, { jailRolls: jailRolls }) });
		if (jailRolls >= MAX_JAIL_ROLLS) {
			const passed = nextTurn(game, now);
			return answer(passed.game, events.concat(passed.events), dice);
		}
		return answer(game, events, dice);
	}
	game = merge(game, { turn: merge(game.turn, { dice: dice, used: [false, false] }) });
	if (legalMoves(game, id).length === 0) {
		const ended = endDice(game, now);
		return answer(ended.game, events.concat(ended.events), dice);
	}
	return answer(game, events, dice);
}

// move moves one of the current player's pieces with an unused die or the
// sum of both.
function move(state, payload, now) {
	const turn = state.turn;
	if (turn.dice === null) {
		return refused(state, "ROLL_FIRST");
	}
	const piece = payload.piece;
	if (!Number.isInteger(piece) || piece < 0 || piece >= PIECES) {
		return refused(state, "NO_SUCH_PIECE");
	}
	const steps = stepsOf(turn, payload.die);
	if (steps === 0) {
		return refused(state, "DIE_USED");
	}
	const id = turn.player;
	const player = state.players[id];
	const from = player.pieces[piece];
	if (from === JAIL || from === GOAL) {
		return refused(state, "PIECE_CANNOT_MOVE");
	}
	const to = from + steps;
	if (to > GOAL) {
		return refused(state, "TOO_FAR");
	}
	const used = [turn.used[0] || payload.die !== "b", turn.used[1] || payload.die !== "a"];
	let game = withPiece(merge(state, { turn: merge(turn, { used: used }) }), id, piece, to);
	let events = [{ event: { type: "MOVED", player: id, piece: piece, from: from, to: to } }];
	if (to <= LAP) {
		const captured = capture(game, id, square(player.seat, to), false);
		game = captured.game;
		events = events.concat(captured.events);
	}
	if (to === GOAL) {
		events = events.concat([{ event: { type: "GOAL", player: id, piece: piece } }]);
		if (won(game.players[id])) {
			return finish(game, id, events, null);
		}
	}
	if ((used[0] && used[1]) || legalMoves(game, id).length === 0) {
		const ended = endDice(game, now);
		return answer(ended.game, events.concat(ended.events), null);
	}
	return answer(game, events, null);
}

// capture sends to jail every other player's piece on ring square at; a safe
// square protects them unless force.
function capture(game, mover, at, force) {
	if (!force && isSafe(at)) {
		return { game: game, events: [] };
	}
	let next = game;
	let events = [];
	for (const id of game.order) {
		const victim = game.players[id];
		if (id === mover) {
			continue;
		}
		const pieces = victim.pieces.map(function (progress) {
			return progress >= 0 && progress <= LAP && square(victim.seat, progress) === at ? JAIL : progress;
		});
		for (let piece = 0; piece < PIECES; piece += 1) {
			if (pieces[piece] !== victim.pieces[piece]) {
				events = events.concat([{ event: { type: "CAPTURED", player: mover, victim: id, piece: piece } }]);
			}
		}
		next = withPlayer(next, id, merge(victim, { pieces: pieces }));
	}
	return { game: next, events: events };
}

// legalMoves lists every move the participant may make with the dice left.
function legalMoves(game, participant) {
	const turn = game.turn;
	if (turn.dice === null) {
		return [];
	}
	const options = [
		{ die: "a", steps: stepsOf(turn, "a") },
		{ die: "b", steps: stepsOf(turn, "b") },
		{ die: "sum", steps: stepsOf(turn, "sum") },
	].filter(function (option) { return option.steps > 0; });
	const pieces = game.players[participant].pieces;
	let moves = [];
	for (let piece = 0; piece < PIECES; piece += 1) {
		const from = pieces[piece];
		if (from === JAIL || from === GOAL) {
			continue;
		}
		const fitting = options.filter(function (option) { return from + option.steps <= GOAL; });
		moves = moves.concat(fitting.map(function (option) {
			return { piece: piece, die: option.die, to: from + option.steps };
		}));
	}
	return moves;
}

// endDice closes the current roll: pairs roll again, anything else passes the
// turn.
function endDice(game, now) {
	const pairs = game.turn.dice[0] === game.turn.dice[1];
	const cleared = merge(game, { turn: merge(game.turn, { dice: null, used: [false, false] }) });
	return pairs ? { game: cleared, events: [] } : nextTurn(cleared, now);
}

// nextTurn hands the turn to the next connected player in order, or to the
// next one when nobody is connected.
function nextTurn(game, now) {
	const from = game.order.indexOf(game.turn.player);
	let next = null;
	for (let offset = 1; offset <= game.order.length && next === null; offset += 1) {
		const id = game.order[(from + offset) % game.order.length];
		if (game.players[id].connected) {
			next = id;
		}
	}
	if (next === null) {
		next = game.order[(from + 1) % game.order.length];
	}
	return { game: merge(game, { turn: newTurn(next, now) }), events: [{ event: { type: "TURN", player: next } }] };
}

function newTurn(player, now) {
	return { player: player, dice: null, used: [false, false], pairs: 0, jailRolls: 0, deadline: now + TURN_MS };
}

function finish(game, winner, events, dice) {
	const done = merge(game, { winner: winner, turn: merge(game.turn, { dice: null }) });
	const answered = answer(done, events.concat([{ event: { type: "WINNER", player: winner } }]), dice);
	return { state: answered.state, response: answered.response, events: answered.events, finished: true };
}

function answer(game, events, dice) {
	return { state: game, response: dice === null ? { ok: true } : { ok: true, dice: dice }, events: events };
}

function refused(state, reason) {
	return { state: state, response: { ok: false, reason: reason } };
}

// stepsOf is how far die moves with the dice left, or 0 when it is used.
function stepsOf(turn, die) {
	if (die === "a") {
		return turn.used[0] ? 0 : turn.dice[0];
	}
	if (die === "b") {
		return turn.used[1] ? 0 : turn.dice[1];
	}
	return turn.used[0] || turn.used[1] ? 0 : turn.dice[0] + turn.dice[1];
}

// square is the ring square a seat's piece stands on at progress, counted
// from the seat's exit.
function square(seat, progress) {
	return (seat * SIDE + EXIT + progress) % RING;
}

function isSafe(at) {
	const local = at % SIDE;
	return local === 3 || local === ENTRY || local === EXIT;
}

// seatsFor spreads count players over the four sides, two face to face.
function seatsFor(count) {
	return count === 2 ? [0, 2] : [0, 1, 2, 3].slice(0, count);
}

// leadingPiece is the index of the most advanced piece out of jail and short
// of the cielo, or null when there is none.
function leadingPiece(player) {
	let best = null;
	for (let piece = 0; piece < PIECES; piece += 1) {
		const progress = player.pieces[piece];
		if (progress !== JAIL && progress !== GOAL && (best === null || progress > player.pieces[best])) {
			best = piece;
		}
	}
	return best;
}

function won(player) {
	return player.pieces.every(function (progress) { return progress === GOAL; });
}

function rollDie() {
	return 1 + Math.floor(Math.random() * 6);
}

// withPiece is game with one of a player's pieces at progress.
function withPiece(game, id, piece, progress) {
	const player = game.players[id];
	const pieces = player.pieces.map(function (current, index) { return index === piece ? progress : current; });
	return withPlayer(game, id, merge(player, { pieces: pieces }));
}

function withPlayer(game, id, player) {
	const change = {};
	change[id] = player;
	return merge(game, { players: merge(game.players, change) });
}

// merge is a new object with base's fields, changes replacing them.
function merge(base, changes) {
	const result = {};
	for (const key of Object.keys(base)) {
		result[key] = base[key];
	}
	for (const key of Object.keys(changes)) {
		result[key] = changes[key];
	}
	return result;
}
`

// seedParquesFrontendPackage is Parqués' frontend package, built with the
// Playhoot Game SDK by the frontend repository (seeddata/README.md).
//
//go:embed seeddata/parques_frontend.html
var seedParquesFrontendPackage []byte

var (
	seedParquesScriptLocator          = objectstore.LocatorFor(backendScriptPrefix, []byte(seedParquesScript))
	seedParquesFrontendPackageLocator = objectstore.LocatorFor(frontendPackagePrefix, seedParquesFrontendPackage)
)

// seed20261020100000Parques adds the Parqués Game and its EXECUTABLE version,
// asked by the human as a turn-based game for the web frontend.
func seed20261020100000Parques() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "seed_20261020100000_parques",
		Migrate: func(tx *gorm.DB) error {
			statements := []struct {
				sql  string
				args []any
			}{
				{sql: `INSERT INTO play_games (game_uuid) VALUES (?)`, args: []any{seedParquesGameUUID}},
				{
					sql: `INSERT INTO play_game_versions (uuid, game_id, status, manifest,
							script_object_key, script_sha256, script_size,
							frontend_object_key, frontend_sha256, frontend_size, executable_at)
						SELECT ?, id, 'EXECUTABLE', ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP FROM play_games WHERE game_uuid = ?`,
					args: []any{
						seedParquesVersionUUID, seedParquesManifest,
						seedParquesScriptLocator.ObjectKey, seedParquesScriptLocator.SHA256, seedParquesScriptLocator.Size,
						seedParquesFrontendPackageLocator.ObjectKey, seedParquesFrontendPackageLocator.SHA256, seedParquesFrontendPackageLocator.Size,
						seedParquesGameUUID,
					},
				},
				{
					sql: `UPDATE play_games SET current_version_id = (SELECT id FROM play_game_versions WHERE uuid = ?)
						WHERE game_uuid = ?`,
					args: []any{seedParquesVersionUUID, seedParquesGameUUID},
				},
			}
			for _, statement := range statements {
				if err := tx.Exec(statement.sql, statement.args...).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				DELETE FROM play_game_versions
				WHERE game_id IN (SELECT id FROM play_games WHERE game_uuid = ?)
			`, seedParquesGameUUID).Error; err != nil {
				return err
			}
			return tx.Exec(`DELETE FROM play_games WHERE game_uuid = ?`, seedParquesGameUUID).Error
		},
	}
}

package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
)

// Space Drift test-data identifiers, so a person testing by hand can create
// its Sessions.
const (
	// seedSpaceDriftGameUUID is a playable Game whose EXECUTABLE version runs
	// seedSpaceDriftScript: ships flying in a shared arena, built to exercise
	// SAMPLE, COMMAND and update() execution and replication to players.
	seedSpaceDriftGameUUID    = "33333333-3333-4333-8333-333333333333"
	seedSpaceDriftVersionUUID = "33333333-3333-4333-8333-0000000000c1"

	// seedSpaceDriftManifest lets one to eight players fly with a move stick
	// and a boost button, and teleport to a point of the arena; update() keeps
	// the world moving between inputs.
	seedSpaceDriftManifest = `{"schema_version":1,"players":{"min":1,"max":8},"entrypoints":["update"],` +
		`"inputs":{"move":{"type":"vector2"},"boost":{"type":"boolean"}},` +
		`"commands":{"teleport":{"payload":{"x":{"type":"number","min":20,"max":980,"default":500},"y":{"type":"number","min":20,"max":980,"default":500}}}}}`
)

// seedSpaceDriftScript is Space Drift's backend script, test content rather
// than a real game; its opening comment explains the rules.
const seedSpaceDriftScript = `// Space Drift: each ship accelerates along its move stick (twice as hard while
// boosting), slows down by drag and bounces off the arena walls; flying through
// the star scores a point and moves the star somewhere random. The teleport
// command moves a ship to a point and stops it, at most once every three
// seconds: earlier, the game answers COOLDOWN and nothing changes. update()
// keeps the world moving while nobody sends anything; the state records how
// far it was simulated and every operation integrates up to its time. A
// disconnected player's ship stops and stays, announced as SHIP_ADRIFT.
const ARENA_SIZE = 1000;
const THRUST = 400;
const BOOST_FACTOR = 2;
const DRAG = 1.5;
const MAX_SPEED = 600;
const WALL_BOUNCE = 0.5;
const SHIP_RADIUS = 20;
const STAR_RADIUS = 15;
const STEP_MS = 20;
const MAX_STEPS = 500;
const TELEPORT_COOLDOWN_MS = 3000;

function start(participants, context) {
	const ships = {};
	participants.forEach(function (participant, index) {
		const angle = (2 * Math.PI * index) / participants.length;
		ships[participant] = {
			x: ARENA_SIZE / 2 + Math.cos(angle) * 300,
			y: ARENA_SIZE / 2 + Math.sin(angle) * 300,
			vx: 0,
			vy: 0,
			thrust: [0, 0],
			boost: false,
			score: 0,
			teleportReadyAt: 0,
		};
	});
	return { state: { simulatedThrough: context.sessionTime, ships: ships, star: spawnStar(), collected: 0 } };
}

function samples(state, transitions, context) {
	const world = copyWorld(state);
	for (const transition of transitions) {
		simulate(world, transition.sessionTime);
		const ship = world.ships[transition.participant];
		ship.thrust = clampStick(transition.inputs.move);
		ship.boost = transition.inputs.boost;
	}
	simulate(world, context.sessionTime);
	return { state: world };
}

function update(state, context) {
	const world = copyWorld(state);
	simulate(world, context.sessionTime);
	return { state: world };
}

function command(state, cmd, context) {
	const world = copyWorld(state);
	simulate(world, cmd.sessionTime);
	const ship = world.ships[cmd.participant];
	if (cmd.sessionTime < ship.teleportReadyAt) {
		return { state: world, response: { ok: false, reason: "COOLDOWN", retryInMs: ship.teleportReadyAt - cmd.sessionTime } };
	}
	ship.x = cmd.payload.x;
	ship.y = cmd.payload.y;
	ship.vx = 0;
	ship.vy = 0;
	ship.teleportReadyAt = cmd.sessionTime + TELEPORT_COOLDOWN_MS;
	return { state: world, response: { ok: true, x: ship.x, y: ship.y } };
}

function playerDisconnected(state, participant, context) {
	const world = copyWorld(state);
	simulate(world, context.sessionTime);
	const ship = world.ships[participant];
	ship.vx = 0;
	ship.vy = 0;
	return { state: world, events: [{ event: { type: "SHIP_ADRIFT", ship: participant } }] };
}

function project(state, participant, context) {
	const me = state.ships[participant];
	const others = {};
	for (const id of Object.keys(state.ships)) {
		if (id !== participant) {
			const ship = state.ships[id];
			others[id] = { x: round(ship.x), y: round(ship.y), score: ship.score };
		}
	}
	return {
		arena: ARENA_SIZE,
		simulatedThrough: state.simulatedThrough,
		me: {
			x: round(me.x), y: round(me.y), vx: round(me.vx), vy: round(me.vy), boost: me.boost, score: me.score,
			teleportReadyAt: me.teleportReadyAt,
		},
		others: others,
		star: { x: round(state.star.x), y: round(state.star.y) },
		collected: state.collected,
	};
}

// simulate advances every ship from world.simulatedThrough to until, in
// fixed steps (longer ones for long gaps, so catching up stays cheap).
function simulate(world, until) {
	const gap = until - world.simulatedThrough;
	if (gap <= 0) {
		return;
	}
	const stepMs = Math.max(STEP_MS, Math.ceil(gap / MAX_STEPS));
	while (world.simulatedThrough < until) {
		const ms = Math.min(stepMs, until - world.simulatedThrough);
		for (const id of Object.keys(world.ships)) {
			step(world, world.ships[id], ms / 1000);
		}
		world.simulatedThrough += ms;
	}
}

function step(world, ship, dt) {
	const thrust = THRUST * (ship.boost ? BOOST_FACTOR : 1);
	ship.vx += ship.thrust[0] * thrust * dt;
	ship.vy += ship.thrust[1] * thrust * dt;
	const keep = Math.max(0, 1 - DRAG * dt);
	ship.vx *= keep;
	ship.vy *= keep;
	const speed = Math.hypot(ship.vx, ship.vy);
	if (speed > MAX_SPEED) {
		ship.vx = (ship.vx / speed) * MAX_SPEED;
		ship.vy = (ship.vy / speed) * MAX_SPEED;
	}
	ship.x += ship.vx * dt;
	ship.y += ship.vy * dt;
	if (ship.x < SHIP_RADIUS || ship.x > ARENA_SIZE - SHIP_RADIUS) {
		ship.x = Math.min(Math.max(ship.x, SHIP_RADIUS), ARENA_SIZE - SHIP_RADIUS);
		ship.vx = -ship.vx * WALL_BOUNCE;
	}
	if (ship.y < SHIP_RADIUS || ship.y > ARENA_SIZE - SHIP_RADIUS) {
		ship.y = Math.min(Math.max(ship.y, SHIP_RADIUS), ARENA_SIZE - SHIP_RADIUS);
		ship.vy = -ship.vy * WALL_BOUNCE;
	}
	if (Math.hypot(ship.x - world.star.x, ship.y - world.star.y) < SHIP_RADIUS + STAR_RADIUS) {
		ship.score += 1;
		world.collected += 1;
		world.star = spawnStar();
	}
}

function spawnStar() {
	const margin = 50;
	return {
		x: margin + Math.random() * (ARENA_SIZE - 2 * margin),
		y: margin + Math.random() * (ARENA_SIZE - 2 * margin),
	};
}

// clampStick keeps a stick inside the unit circle.
function clampStick(move) {
	const length = Math.hypot(move[0], move[1]);
	return length > 1 ? [move[0] / length, move[1] / length] : [move[0], move[1]];
}

function copyWorld(state) {
	const ships = {};
	for (const id of Object.keys(state.ships)) {
		const ship = state.ships[id];
		ships[id] = {
			x: ship.x, y: ship.y, vx: ship.vx, vy: ship.vy,
			thrust: [ship.thrust[0], ship.thrust[1]], boost: ship.boost, score: ship.score,
			teleportReadyAt: ship.teleportReadyAt,
		};
	}
	return { simulatedThrough: state.simulatedThrough, ships: ships, star: { x: state.star.x, y: state.star.y }, collected: state.collected };
}

function round(value) {
	return Math.round(value * 100) / 100;
}
`

var seedSpaceDriftScriptLocator = objectstore.LocatorFor(backendScriptPrefix, []byte(seedSpaceDriftScript))

// seed20261018000000SpaceDrift adds the Space Drift Game and its EXECUTABLE
// version, sharing the seed frontend package.
func seed20261018000000SpaceDrift() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "seed_20261018000000_space_drift",
		Migrate: func(tx *gorm.DB) error {
			statements := []struct {
				sql  string
				args []any
			}{
				{sql: `INSERT INTO play_games (game_uuid) VALUES (?)`, args: []any{seedSpaceDriftGameUUID}},
				{
					sql: `INSERT INTO play_game_versions (uuid, game_id, status, manifest,
							script_object_key, script_sha256, script_size,
							frontend_object_key, frontend_sha256, frontend_size, executable_at)
						SELECT ?, id, 'EXECUTABLE', ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP FROM play_games WHERE game_uuid = ?`,
					args: []any{
						seedSpaceDriftVersionUUID, seedSpaceDriftManifest,
						seedSpaceDriftScriptLocator.ObjectKey, seedSpaceDriftScriptLocator.SHA256, seedSpaceDriftScriptLocator.Size,
						seedFrontendPackageLocator.ObjectKey, seedFrontendPackageLocator.SHA256, seedFrontendPackageLocator.Size,
						seedSpaceDriftGameUUID,
					},
				},
				{
					sql: `UPDATE play_games SET current_version_id = (SELECT id FROM play_game_versions WHERE uuid = ?)
						WHERE game_uuid = ?`,
					args: []any{seedSpaceDriftVersionUUID, seedSpaceDriftGameUUID},
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
			`, seedSpaceDriftGameUUID).Error; err != nil {
				return err
			}
			return tx.Exec(`DELETE FROM play_games WHERE game_uuid = ?`, seedSpaceDriftGameUUID).Error
		},
	}
}

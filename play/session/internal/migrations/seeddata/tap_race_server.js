"use strict";
//#region packages/game-server/src/index.ts
function defineGame(definition) {
	return definition;
}

//#endregion
//#region packages/game-core/src/index.ts
function defineCommands(declared) {
	const result = {};
	for (const name of Object.keys(declared)) result[name] = {
		name,
		payload: declared[name] ?? {}
	};
	return result;
}
function manifestField(field) {
	if (typeof field === "string") return { type: field };
	if (Array.isArray(field)) {
		const values = [...field];
		return {
			type: "enum",
			values,
			default: values[0]
		};
	}
	const declared = field;
	const result = { type: declared.type };
	if (declared.type === "enum") {
		result.values = [...declared.values];
		result.default = declared.default ?? declared.values[0];
	}
	if (declared.type === "number") {
		if (declared.min !== void 0) result.min = declared.min;
		if (declared.max !== void 0) result.max = declared.max;
	}
	if (declared.type === "string" && declared.nullable) result.nullable = true;
	if (declared.type !== "enum" && declared.default !== void 0) result.default = declared.default;
	return result;
}
function manifestFields(fields) {
	const result = {};
	for (const name of Object.keys(fields)) {
		const field = fields[name];
		if (field !== void 0) result[name] = manifestField(field);
	}
	return result;
}

//#endregion
//#region fixtures/sdk-games/tap-race/shared/actions.ts
const commands = defineCommands({ tap: {} });

//#endregion
//#region fixtures/sdk-games/tap-race/server/game.ts
const GOAL = 20;
const DURATION_MS = 3e4;
const game_default = defineGame({
	players: {
		min: 1,
		max: 8
	},
	commands,
	setup: (players) => ({
		taps: Object.fromEntries(players.map((player) => [player, 0])),
		winner: null
	}),
	on: { tap(state, { player, finish }) {
		state.taps[player] += 1;
		if (state.taps[player] >= GOAL) {
			state.winner = player;
			finish();
		}
	} },
	update(state, { now, finish }) {
		if (now < DURATION_MS) return;
		state.winner = leader(state);
		finish();
	},
	view: (state, player, { now }) => ({
		me: player,
		taps: state.taps,
		goal: GOAL,
		secondsLeft: Math.max(0, Math.ceil((DURATION_MS - now) / 1e3)),
		winner: state.winner
	})
});
function leader(state) {
	let best = null;
	for (const [player, taps] of Object.entries(state.taps)) if (best === null || taps > state.taps[best]) best = player;
	return best;
}

//#endregion
//#region packages/game-server/src/runtime.ts
function gameContext(output, now, elapsed) {
	return {
		now,
		elapsed,
		emit(event, to) {
			if (to === void 0) output.events.push({ event });
			else output.events.push({
				event,
				participants: typeof to === "string" ? [to] : [...to]
			});
		},
		error(player, error) {
			output.errors.push({
				participant: player,
				error
			});
		},
		finish() {
			output.finished = true;
		}
	};
}
function newOutput() {
	return {
		events: [],
		errors: [],
		finished: false
	};
}
function result(state, output, extra) {
	const answer = {
		state,
		...extra
	};
	if (output.events.length > 0) answer.events = output.events;
	if (output.errors.length > 0) answer.errors = output.errors;
	if (output.finished) answer.finished = true;
	return answer;
}
function isRejection(value) {
	return typeof value === "object" && value !== null && typeof value.rejected === "string";
}
function __playhoot_start(game, participants) {
	return { state: game.setup(participants) };
}
function __playhoot_command(game, state, command, context) {
	const output = newOutput();
	const handler = game.on?.[command.name];
	if (handler === void 0) throw new Error(`the game has no handler for command ${command.name}`);
	const answer = handler(state, {
		...gameContext(output, command.sessionTime, context.elapsed),
		player: command.participant,
		payload: command.payload
	});
	let response = { ok: true };
	if (isRejection(answer)) response = answer.details === void 0 ? {
		ok: false,
		code: answer.rejected
	} : {
		ok: false,
		code: answer.rejected,
		details: answer.details
	};
	else if (answer !== void 0) response = {
		ok: true,
		value: answer
	};
	return result(state, output, { response });
}
function __playhoot_samples(game, state, transitions, context) {
	const output = newOutput();
	for (const transition of transitions) game.input?.(state, {
		...gameContext(output, transition.sessionTime, context.elapsed),
		player: transition.participant,
		inputs: transition.inputs
	});
	return result(state, output);
}
function __playhoot_update(game, state, context) {
	const output = newOutput();
	game.update?.(state, gameContext(output, context.sessionTime, context.elapsed));
	return result(state, output);
}
function __playhoot_player_disconnected(game, state, participant, context) {
	const output = newOutput();
	game.leave?.(state, {
		...gameContext(output, context.sessionTime, context.elapsed),
		player: participant
	});
	return result(state, output);
}
function __playhoot_player_rejoined(game, state, participant, context) {
	const output = newOutput();
	return result(state, output, { accepted: (game.rejoin?.(state, {
		...gameContext(output, context.sessionTime, context.elapsed),
		player: participant
	}) ?? false) === true });
}
function __playhoot_project(game, state, participant, context) {
	return game.view(state, participant, { now: context.sessionTime });
}
function __playhoot_manifest_of(game) {
	const commands = {};
	const declared = game.commands ?? {};
	for (const name of Object.keys(declared)) commands[name] = { payload: manifestFields(declared[name]?.payload ?? {}) };
	const entrypoints = [];
	if (game.update !== void 0) entrypoints.push("update");
	if (game.rejoin !== void 0) entrypoints.push("playerRejoined");
	return {
		schema_version: 1,
		players: {
			min: game.players.min,
			max: game.players.max
		},
		inputs: manifestFields(game.inputs ?? {}),
		commands,
		entrypoints
	};
}

//#endregion

//#region generated by @playhoot/game-tools
function start(participants, context) { return __playhoot_start(game_default, participants); }
function command(state, cmd, context) { return __playhoot_command(game_default, state, cmd, context); }
function samples(state, transitions, context) { return __playhoot_samples(game_default, state, transitions, context); }
function update(state, context) { return __playhoot_update(game_default, state, context); }
function playerDisconnected(state, participant, context) { return __playhoot_player_disconnected(game_default, state, participant, context); }
function playerRejoined(state, participant, context) { return __playhoot_player_rejoined(game_default, state, participant, context); }
function project(state, participant, context) { return __playhoot_project(game_default, state, participant, context); }
function __playhoot_manifest() { return __playhoot_manifest_of(game_default); }
function __playhoot_module_values() { return [commands, game_default]; }
//#endregion

import type { METHODS as DECLARED } from "../package/io.github.tangerg.flame/contracts/methods.generated";

// The Go contract owns these names. `satisfies` fails the build when a declared
// version moves, so a method bump touches only this table.
export const METHODS = Object.freeze({
  create: "flame.schedules/create.v2",
  delete: "flame.schedules/delete.v2",
  list: "flame.schedules/list.v2",
  runNow: "flame.schedules/runNow.v2",
  update: "flame.schedules/update.v2",
} as const satisfies Record<string, keyof typeof DECLARED>);

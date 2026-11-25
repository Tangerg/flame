import type { METHODS as DECLARED } from "../package/io.github.tangerg.flame/contracts/methods.generated";

// The Go contract owns these names. `satisfies` fails the build when a declared
// version moves, so a method bump touches only this table.
export const METHODS = Object.freeze({
  add: "flame.memory/add.v3",
  delete: "flame.memory/delete.v3",
  list: "flame.memory/list.v3",
  review: "flame.memory/review.v3",
  update: "flame.memory/update.v3",
} as const satisfies Record<string, keyof typeof DECLARED>);

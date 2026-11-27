import type { METHODS as DECLARED } from "../package/io.github.tangerg.flame/contracts/methods.generated";

// The Go contract owns these names. `satisfies` fails the build when a declared
// version moves, so a method bump touches only this table.
export const METHODS = Object.freeze({
  create: "flame.schedules/create.v2",
  delete: "flame.schedules/delete.v3",
  list: "flame.schedules/list.v2",
  runNow: "flame.schedules/runNow.v2",
  update: "flame.schedules/update.v2",
} as const satisfies Record<string, keyof typeof DECLARED>);

export const AUTHORING_LABELS = {
  [METHODS.create]: "command.create",
  [METHODS.update]: "command.save",
  [METHODS.delete]: "command.delete",
  [METHODS.runNow]: "command.runNow",
} as const;

export type AuthoringMethod = keyof typeof AUTHORING_LABELS;

export function isAuthoringMethod(method: string): method is AuthoringMethod {
  return Object.hasOwn(AUTHORING_LABELS, method);
}

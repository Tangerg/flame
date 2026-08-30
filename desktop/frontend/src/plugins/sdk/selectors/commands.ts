// Only the reads with real logic — imperative execution, slash-command key pairing, owner
// attribution. A plain read goes through the generic substrate.

import { useMemo } from "react";
import type { SlashCommandSpec } from "../types";
import { COMMAND, SLASH_COMMAND } from "../kernelPoints";
import { lookupExtensionByKey, lookupExtensionOwner, useExtensionEntries } from "./extensions";

/** Warns and no-ops when nothing matches. */
export function executeCommand(id: string, ...args: unknown[]): Promise<void> {
  const command = lookupExtensionByKey(COMMAND, id);
  if (!command) {
    console.warn(`[plugin] commands.execute("${id}"): no command registered`);
    return Promise.resolve();
  }
  return Promise.resolve(command.run(...args));
}

// The slash-command trigger lives in the map key, not on the spec, so the
// generic read can't surface it — we pair key+spec into a list.

export function useSlashCommands(): Array<{ cmd: string; spec: SlashCommandSpec }> {
  const entries = useExtensionEntries(SLASH_COMMAND);
  return useMemo(() => entries.map((entry) => ({ cmd: entry.key, spec: entry.item })), [entries]);
}

export function lookupSlashCommandOwner(cmd: string): string | undefined {
  return lookupExtensionOwner(SLASH_COMMAND, cmd);
}

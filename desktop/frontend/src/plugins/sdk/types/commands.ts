/**
 * An action invokable by id; one carrying a combo is also projected into the global shortcut
 * registry. Distinct from a slash command — register both to reach an action from both UIs.
 */
export interface CommandSpec {
  id: string;
  /**
   * A catalog KEY, resolved where the shortcut renders — never resolved text. A command is
   * registered once and nothing re-registers on a language switch, so `t(...)` here freezes
   * the label in the boot locale. An unknown key renders as itself, keeping the identifier
   * visible rather than showing an empty label.
   */
  label: string;
  /** e.g. "Mod+N" — Cmd on Mac, Ctrl elsewhere. */
  combo?: string;
  /** Shortcut triggers pass NO args, so most commands take zero params. */
  run: (...args: unknown[]) => void | Promise<void>;
}

/** Receives the raw event so the handler can decide whether to `preventDefault`. */
export type ShortcutHandler = (event: KeyboardEvent) => void;

/**
 * `key` is a `KeyboardEvent.key` with optional `+`-joined modifiers; prefer "Mod" over
 * "Cmd"/"Ctrl". Matching is case-insensitive, and the LAST registration of a combo wins.
 */
export interface ShortcutSpec {
  key: string;
  handler: ShortcutHandler;
  /** A catalog key, displayed in the shortcuts settings pane. */
  description?: string;
  /** Defaults to false: most shortcuts must not steal typing input. */
  allowInInputs?: boolean;
}

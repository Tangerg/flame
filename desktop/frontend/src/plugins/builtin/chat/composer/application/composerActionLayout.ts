type ComposerAction = "send" | "steer" | "stop";

export interface ComposerActionLayout {
  primary: ComposerAction;
  secondary: "stop" | null;
}

/**
 * The rule this exists to hold: while a run is in flight, STOP is always reachable, so typing
 * a steer never takes the stop button's place.
 */
export function composerActionLayout({
  running,
  hasInput,
}: {
  running: boolean;
  hasInput: boolean;
}): ComposerActionLayout {
  if (!running) return { primary: "send", secondary: null };
  if (hasInput) return { primary: "steer", secondary: "stop" };
  return { primary: "stop", secondary: null };
}

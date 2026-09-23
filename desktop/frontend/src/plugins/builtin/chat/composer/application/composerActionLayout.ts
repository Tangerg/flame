type ComposerAction = "send" | "steer" | "stop";

export interface ComposerActionLayout {
  primary: ComposerAction;
  secondary: "stop" | null;
}

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

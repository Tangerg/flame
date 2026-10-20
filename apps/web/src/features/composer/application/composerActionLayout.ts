export interface ComposerActionLayout {
  submit: "send" | "steer" | null;
  stop: "emphasized" | "quiet" | null;
}

export function composerActionLayout({
  running,
  hasInput,
}: {
  running: boolean;
  hasInput: boolean;
}): ComposerActionLayout {
  if (!running) return { submit: "send", stop: null };
  if (hasInput) return { submit: "steer", stop: "quiet" };
  return { submit: null, stop: "emphasized" };
}

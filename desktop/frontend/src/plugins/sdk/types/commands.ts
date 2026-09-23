export interface CommandSpec {
  id: string;
  label: string;
  combo?: string;
  run: (...args: unknown[]) => void | Promise<void>;
}

export type ShortcutHandler = (event: KeyboardEvent) => void;

export interface ShortcutSpec {
  key: string;
  handler: ShortcutHandler;
  description?: string;
  allowInInputs?: boolean;
}

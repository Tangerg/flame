import type { ComponentType } from "react";
import type { CapturedSession } from "@flame/plugin-sdk";

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

export interface SessionActionSpec {
  id: string;
  title: string;
  component: ComponentType<{ session: CapturedSession }>;
}

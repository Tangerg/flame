import type { NotificationEntry, NotificationLevel } from "@/features/notifications/public/types";
import { createStore } from "zustand/vanilla";
import { ExactSequence } from "@/foundation/exactSequence";

const MAX_ENTRIES = 200;

export type NotifySource =
  | "agentMemory"
  | "composer"
  | "events"
  | "goal"
  | "import"
  | "mcp"
  | "project"
  | "render"
  | "session"
  | "setup"
  | "skills";

interface NotificationPresentation {
  level: NotificationLevel | "success";
  message: string;
  description?: string;
  lifetime?: "persistent";
  action?: { label: string; run(): void };
}

export class NotificationFeed {
  readonly #history = createStore(() => ({ log: [] as NotificationEntry[] }));
  readonly #ids = new ExactSequence();
  readonly #present: (message: NotificationPresentation) => void;
  #active = true;

  constructor(present: (message: NotificationPresentation) => void) {
    this.#present = present;
  }

  readonly snapshot = (): readonly NotificationEntry[] => this.#history.getState().log;
  readonly subscribe = (listener: () => void): (() => void) => this.#history.subscribe(listener);

  record(input: {
    plugin: string;
    level: NotificationLevel;
    message: string;
  }): NotificationEntry | undefined {
    if (!this.#active) return;
    const entry: NotificationEntry = {
      ...input,
      id: this.#ids.issue().toString(),
      timestamp: Date.now(),
    };
    this.#history.setState(({ log }) => ({ log: [...log, entry].slice(-MAX_ENTRIES) }));
    return entry;
  }

  notify(
    level: NotificationLevel,
    message: string,
    options?: { description?: string; source?: string },
  ): void {
    if (!this.#active) return;
    this.record({
      plugin: options?.source ?? "app",
      level,
      message: options?.description ? `${message} — ${options.description}` : message,
    });
    this.show({ level, message, description: options?.description });
  }

  readonly show = (message: NotificationPresentation): void => {
    if (!this.#active) return;
    const action = message.action;
    this.#present({
      ...message,
      action: action && {
        label: action.label,
        run: () => {
          if (this.#active) action.run();
        },
      },
    });
  };

  readonly dismiss = (id: string): void => {
    if (!this.#active) return;
    this.#history.setState(({ log }) => ({
      log: log.map((entry) => (entry.id === id ? { ...entry, dismissed: true } : entry)),
    }));
  };

  readonly clearAll = (): void => {
    if (this.#active) this.#history.setState({ log: [] });
  };

  dispose(): void {
    this.#active = false;
  }
}

import { service } from "dougong";

export interface WindowService {
  setTitle(text: string): void;
  setBadge(n?: number): void;
  setWorking(on: boolean): void;
}

export class WindowTitle implements WindowService {
  readonly #defaultTitle: string;
  #title: string;
  #badge = 0;
  #working = false;

  constructor(defaultTitle: string) {
    this.#defaultTitle = defaultTitle;
    this.#title = defaultTitle;
  }

  setTitle(text: string): void {
    this.#title = text || this.#defaultTitle;
    this.#publish();
  }

  setBadge(count = 0): void {
    this.#badge = Math.max(0, count);
    this.#publish();
  }

  setWorking(working: boolean): void {
    this.#working = working;
    this.#publish();
  }

  #publish(): void {
    if (typeof document === "undefined") return;
    const activity = this.#working ? "● " : "";
    const badge = this.#badge > 0 ? `(${this.#badge}) ` : "";
    document.title = `${activity}${badge}${this.#title}`;
  }
}

export const WINDOW = service<WindowService>("flame.shell.window");

import { failureText } from "@flame/component-sdk";
import { type Page } from "./types";

interface PageSelection {
  readonly cursors: readonly string[];
  readonly focusFrom?: HTMLElement;
}

interface AcceptedPage extends PageSelection {
  readonly page: Page;
}

export type ScheduleCatalogRead =
  | { loading: true; accepted?: AcceptedPage; error?: never }
  | { loading: false; accepted: AcceptedPage; error?: never }
  | { loading: false; accepted?: AcceptedPage; error: string };

export class ScheduleCatalog {
  // Invalidation follows the selected cursor while the last accepted page stays on screen.
  #selection: PageSelection = { cursors: [] };
  #read: ScheduleCatalogRead = { loading: true };
  #reading?: AbortController;
  readonly #listeners = new Set<() => void>();

  constructor(
    private readonly list: (cursor: string, signal: AbortSignal) => Promise<Page>,
    private readonly signal: AbortSignal,
  ) {}

  snapshot = () => this.#read;

  subscribe = (listener: () => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  #publish(read: ScheduleCatalogRead) {
    this.#read = read;
    for (const listener of this.#listeners) listener();
  }

  stop = () => this.#reading?.abort();

  openPage = (cursors: readonly string[], focusFrom: HTMLElement) => {
    this.#selection = { cursors: [...cursors], focusFrom };
    return this.refresh();
  };

  refresh = async () => {
    if (this.signal.aborted) return;
    this.stop();
    const opening = this.#selection;
    const request = new AbortController();
    this.#reading = request;
    const signal = AbortSignal.any([this.signal, request.signal]);
    const accepted = this.#read.accepted;
    this.#publish({ loading: true, accepted });
    let settled: ScheduleCatalogRead;
    try {
      const page = await this.list(opening.cursors.at(-1) ?? "", signal);
      settled = { loading: false, accepted: { ...opening, page } };
    } catch (error) {
      settled = { loading: false, accepted, error: failureText(error) };
    } finally {
      if (this.#reading === request) this.#reading = undefined;
    }
    if (signal.aborted) return;
    this.#selection = { cursors: opening.cursors };
    this.#publish(settled);
  };
}

import { failureText } from "@flame/component-sdk";
import { parseTarget } from "./targets";

export interface Selection {
  readonly id: string;
  readonly revision: string;
}

interface DocumentSelection extends Selection {
  readonly object?: string;
}

interface NavigationSnapshot {
  readonly selection?: DocumentSelection;
  readonly linkError?: string;
}

function sameSelection(a?: Selection, b?: Selection): boolean {
  return a?.id === b?.id && a?.revision === b?.revision;
}

export class DocumentNavigation {
  #view: NavigationSnapshot = {};
  // A head read can outlive the link that requested it.
  #pendingLink?: AbortController;
  readonly #listeners = new Set<() => void>();

  constructor(
    private readonly signal: AbortSignal,
    private readonly readHead: (id: string, signal: AbortSignal) => Promise<string>,
    private readonly changed: (previous?: Selection, next?: Selection) => void,
  ) {}

  snapshot = () => this.#view;

  subscribe = (listener: () => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  open = ({ id, revision }: Selection) => {
    if (this.signal.aborted) return;
    this.#retireLink();
    this.#select({ id, revision });
  };

  replace(previous: Selection | undefined, next: Selection) {
    if (
      !this.signal.aborted &&
      !this.#pendingLink &&
      sameSelection(this.#view.selection, previous)
    ) {
      this.#select(next);
    }
  }

  clear = (previous: Selection) => {
    if (!this.signal.aborted && sameSelection(this.#view.selection, previous))
      this.#select();
  };

  async openLink(value: string) {
    if (this.signal.aborted) return;
    this.#retireLink();
    const target = parseTarget(value);
    if (!target) {
      this.#publish({ ...this.#view, linkError: value });
      return;
    }
    const request = new AbortController();
    this.#pendingLink = request;
    const signal = AbortSignal.any([this.signal, request.signal]);
    try {
      const revision = target.revision ?? (await this.readHead(target.document, signal));
      if (!signal.aborted)
        this.#select({ id: target.document, revision, object: target.object });
    } catch (error) {
      if (!signal.aborted)
        this.#publish({ ...this.#view, linkError: failureText(error) });
    } finally {
      if (this.#pendingLink === request) this.#pendingLink = undefined;
    }
  }

  #retireLink() {
    this.#pendingLink?.abort();
    this.#pendingLink = undefined;
  }

  #select(selection?: DocumentSelection) {
    const previous = this.#view.selection;
    if (!sameSelection(previous, selection)) this.changed(previous, selection);
    this.#publish({ selection });
  }

  #publish(view: NavigationSnapshot) {
    this.#view = view;
    for (const listener of this.#listeners) listener();
  }
}

import { z } from "zod/v4";
import {
  editorEditSchemas,
  STAGED_OPERATIONS,
  type DraftFormat,
  type DraftOperation,
  type EditorOperation,
} from "../contracts/content";
import type { Draft, DraftUpdate } from "./live-draft";

type DraftObservation<Edit> =
  | { kind: "stored"; encoded: string | null; draft: Draft<Edit> | undefined }
  | { kind: "volatile"; draft: Draft<Edit> | undefined };

function unavailable(error: unknown): boolean {
  return (
    error instanceof DOMException &&
    (error.name === "SecurityError" || error.name === "QuotaExceededError")
  );
}

function draftOrAbsent<Edit>(
  draft: Draft<Edit> | undefined,
): Draft<Edit> | undefined {
  return draft?.operations.length || draft?.undo.some((edits) => edits.length)
    ? draft
    : undefined;
}

export class DraftStore<Format extends DraftFormat> {
  readonly #prefix: string;
  readonly #format: Format;
  readonly #schema: z.ZodType<Draft<EditorOperation>>;
  readonly #observed = new Map<
    string,
    DraftObservation<DraftOperation<Format>>
  >();
  readonly #listeners = new Set<() => void>();

  constructor(installation: string, format: Format) {
    this.#prefix = `flame.deliverables.draft.${format}.${installation}.`;
    this.#format = format;
    const operation = editorEditSchemas[format].shape.operations.element;
    this.#schema = z.strictObject({
      base: z.string().regex(/^[a-f0-9]{64}$/u),
      operations: z.array(operation),
      undo: z.array(z.array(operation)),
    });
  }

  subscribe = (listener: () => void): (() => void) => {
    if (!this.#listeners.size)
      window.addEventListener("storage", this.#changed);
    this.#listeners.add(listener);
    return () => {
      this.#listeners.delete(listener);
      if (!this.#listeners.size)
        window.removeEventListener("storage", this.#changed);
    };
  };

  #changed = (event: StorageEvent): void => {
    if (event.key === null || event.key.startsWith(this.#prefix))
      this.#publish();
  };

  #publish(): void {
    for (const listener of this.#listeners) listener();
  }

  get(id: string): Draft<DraftOperation<Format>> | undefined {
    const observed = this.#observed.get(id);
    if (observed?.kind === "volatile") return observed.draft;
    let encoded: string | null;
    try {
      encoded = localStorage.getItem(this.#prefix + id);
    } catch (error) {
      if (!unavailable(error)) throw error;
      return observed?.draft;
    }
    if (observed?.encoded === encoded) return observed.draft;
    const draft = encoded === null ? undefined : this.#decode(encoded);
    this.#observed.set(id, { kind: "stored", encoded, draft });
    return draft;
  }

  #decode(encoded: string): Draft<DraftOperation<Format>> | undefined {
    let value: unknown;
    try {
      value = JSON.parse(encoded);
    } catch (error) {
      if (!(error instanceof SyntaxError)) throw error;
      return undefined;
    }
    const parsed = this.#schema.safeParse(value);
    if (!parsed.success) return undefined;
    const { base, operations, undo } = parsed.data;
    const allowed: readonly string[] = STAGED_OPERATIONS[this.#format];
    const staged = (
      edits: EditorOperation[],
    ): edits is DraftOperation<Format>[] =>
      edits.every((edit) => allowed.includes(edit.type));
    return staged(operations) && undo.every(staged)
      ? draftOrAbsent({ base, operations, undo })
      : undefined;
  }

  update(id: string, mutate: DraftUpdate<DraftOperation<Format>>): void {
    // A command updates the latest value for this document. A render snapshot
    // can outlive a different window's edit or a delayed save acknowledgement.
    const current = this.get(id);
    const draft = draftOrAbsent(mutate(current));
    if (draft === current) return;
    const key = this.#prefix + id;
    const encoded = draft ? JSON.stringify(draft) : null;
    try {
      if (encoded === null) localStorage.removeItem(key);
      else localStorage.setItem(key, encoded);
      this.#observed.set(id, { kind: "stored", encoded, draft });
    } catch (error) {
      if (!unavailable(error)) throw error;
      this.#observed.set(id, { kind: "volatile", draft });
    }
    this.#publish();
  }
}

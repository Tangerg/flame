import { useEffect, useMemo, useRef, useState } from "react";
import { failureText } from "@flame/component-sdk";
import { slideLayoutSchema, type ElementView } from "../contracts/content";
import { METHODS } from "../contracts/methods";
import type { DocumentRead } from "./types";
import type { DeliverablesState } from "./useDeliverables";
import {
  operationsJSON,
  record,
  type Draft,
  type DraftOperation,
} from "./slides-draft";
import { type LayoutRead } from "./slides-presentation";

// The part of the deliverables state this view reads.
export type SlideStudioState = Pick<
  DeliverablesState,
  | "blocked"
  | "context"
  | "open"
  | "operations"
  | "refresh"
  | "run"
  | "selectRevision"
  | "signal"
>;

// The bounded format process admits one operation at a time, so revisions are
// read in sequence rather than in parallel.
export function useLayouts(
  state: SlideStudioState,
  documentId: string,
  wanted: string[],
) {
  const [entries, setEntries] = useState<Record<string, LayoutRead>>({});
  const active = useRef<string | undefined>(undefined);
  const owner = useMemo(() => new AbortController(), []);
  useEffect(() => () => owner.abort(), [owner]);
  const key = wanted.join(" ");
  useEffect(() => {
    const next = wanted.find((revision) => !entries[revision]);
    if (!next || active.current) return;
    active.current = next;
    const lifetime = AbortSignal.any([state.signal, owner.signal]);
    void state.operations
      .query(METHODS.slides, { id: documentId, revision: next }, lifetime)
      .then((result): LayoutRead => {
        if (result.revision !== next)
          throw new Error(
            "deliverables: slide layout differs from its revision",
          );
        return {
          value: slideLayoutSchema.parse(JSON.parse(result.layoutJSON)),
        };
      })
      .catch((error: unknown): LayoutRead => ({ error: failureText(error) }))
      .then((read) => {
        active.current = undefined;
        if (!lifetime.aborted)
          setEntries((current) => ({ ...current, [next]: read }));
      });
  }, [key, entries, documentId, owner, state.operations, state.signal]);
  return {
    get: (revision: string) => entries[revision],
    retry: (revision: string) =>
      setEntries((current) => {
        const { [revision]: _retired, ...rest } = current;
        return rest;
      }),
  };
}

export type DraftUpdate = (current: Draft | undefined) => Draft | undefined;

// The draft records intent against the revision it was read from; edits apply only
// while that revision is the head, and saving re-checks it as the precondition.
export function useSlideDraft(
  state: SlideStudioState,
  document: DocumentRead,
  draft: Draft | undefined,
  onDraft: (update: DraftUpdate) => void,
) {
  const documentId = document.document.id;
  const head = document.document.head;
  const viewed = document.revision.id;
  const setDraft = (next: Draft | undefined | DraftUpdate) =>
    onDraft(typeof next === "function" ? next : () => next);
  const [saving, setSaving] = useState(false);
  const stale = draft !== undefined && draft.base !== viewed;
  useEffect(() => {
    if (stale && !draft.operations.length) setDraft(undefined);
  }, [draft, viewed]);
  const editedDraft = draft?.base === viewed ? draft : undefined;
  const editable =
    viewed === head &&
    !state.blocked &&
    !saving &&
    (!draft || editedDraft !== undefined);
  const dirty = !!draft?.operations.length;

  const save = async () => {
    if (!draft || !dirty || !editable) return;
    const captured = draft;
    setSaving(true);
    try {
      const revision = await state.run(
        METHODS.edit,
        {
          id: documentId,
          expectedRevision: captured.base,
          operationsJSON: operationsJSON(captured.operations),
        },
        (value) => {
          setDraft((current) => (current === captured ? undefined : current));
          state.selectRevision(value, {
            id: documentId,
            revision: captured.base,
          });
        },
      );
      if (!revision) void state.refresh();
    } finally {
      setSaving(false);
    }
  };

  return {
    documentId,
    head,
    viewed,
    setDraft,
    saving,
    pending: stale && draft.operations.length ? draft.base : undefined,
    editedDraft,
    editable,
    dirty,
    change: (operation: DraftOperation, target?: ElementView) => {
      if (!editable) return;
      setDraft((current) =>
        record(
          current ?? { base: viewed, operations: [], undo: [] },
          operation,
          target,
        ),
      );
    },
    undo: () =>
      setDraft((current) =>
        current?.undo.length
          ? {
              base: current.base,
              operations: current.undo.at(-1)!,
              undo: current.undo.slice(0, -1),
            }
          : current,
      ),
    save,
    restructure: (operations: unknown[]) => {
      if (!editable || dirty) return;
      const captured = { id: documentId, revision: viewed };
      void state.run(
        METHODS.edit,
        {
          id: documentId,
          expectedRevision: viewed,
          operationsJSON: JSON.stringify({ operations }),
        },
        (value) => state.selectRevision(value, captured),
      );
    },
  };
}

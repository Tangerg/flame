import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import type { z } from "zod/v4";
import {
  failureText,
  type ComposerReference,
  type Translate,
} from "@flame/component-sdk";
import {
  Badge,
  Button,
  Callout,
  Facts,
  FormActions,
  Metadata,
  RecordGroup,
  Segmented,
  Toolbar,
  copyText,
} from "@flame/ui";
import { METHODS } from "../contracts/methods";
import type { DocumentRead } from "./types";
import type { DeliverablesState } from "./useDeliverables";
import {
  operationsJSON,
  rebase,
  undone,
  type Draft,
  type DraftUpdate,
  type Reconciliation,
} from "./live-draft";

// The part of the deliverables state a live editor reads.
export type LiveState = Pick<
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

export type ProjectionRead<T> = { value?: T; error?: string };

type ProjectionMethod = (typeof METHODS)["slides" | "sheets"];

// The bounded format process admits one operation at a time, so revisions are
// read in sequence rather than in parallel.
export function useProjection<T>(
  state: LiveState,
  documentId: string,
  wanted: string[],
  method: ProjectionMethod,
  schema: z.ZodType<T>,
) {
  const [entries, setEntries] = useState<Record<string, ProjectionRead<T>>>({});
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
      .query(method, { id: documentId, revision: next }, lifetime)
      .then((result): ProjectionRead<T> => {
        if (result.revision !== next)
          throw new Error("deliverables: projection differs from its revision");
        return { value: schema.parse(JSON.parse(result.layoutJSON)) };
      })
      .catch((error: unknown): ProjectionRead<T> => ({
        error: failureText(error),
      }))
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

// The draft records intent against the revision it was read from; edits apply only
// while that revision is the head, and saving re-checks it as the precondition.
export function useLiveDraft<Operation>(
  state: LiveState,
  document: DocumentRead,
  draft: Draft<Operation> | undefined,
  onDraft: (update: DraftUpdate<Operation>) => void,
) {
  const documentId = document.document.id;
  const head = document.document.head;
  const viewed = document.revision.id;
  const setDraft = (
    next: Draft<Operation> | undefined | DraftUpdate<Operation>,
  ) => onDraft(typeof next === "function" ? next : () => next);
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
    apply: (change: (draft: Draft<Operation>) => Draft<Operation>) => {
      if (!editable) return;
      setDraft((current) =>
        change(current ?? { base: viewed, operations: [], undo: [] }),
      );
    },
    undo: () => setDraft((current) => current && undone(current)),
    save,
    // Structural edits form their own revision, so they need an empty draft.
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

export function Reconcile<Operation>({
  results,
  head,
  describe,
  mine,
  theirs,
  withCurrent,
  onRebase,
  onDiscard,
  t,
}: {
  results: Reconciliation<Operation>[];
  head: string;
  describe(operation: Operation): string;
  mine(operation: Operation): string;
  theirs(result: Reconciliation<Operation>): string;
  withCurrent(operation: Operation, current: unknown): Operation;
  onRebase(draft: Draft<Operation>): void;
  onDiscard(): void;
  t: Translate;
}) {
  const [kept, setKept] = useState<ReadonlySet<number>>(new Set());
  return (
    <RecordGroup>
      <Metadata>{t("draft.reconcile.help")}</Metadata>
      {results.map((result, index) => (
        <RecordGroup key={index}>
          <Metadata>
            {describe(result.operation)} ·{" "}
            <Badge>{t(`draft.status.${result.status}`)}</Badge>
          </Metadata>
          {result.status === "conflict" && (
            <>
              <Facts
                values={[
                  [t("draft.mine"), mine(result.operation)],
                  [t("draft.theirs"), theirs(result)],
                ]}
              />
              <Segmented
                ariaLabel={t("draft.reconcile.choice")}
                value={kept.has(index) ? "mine" : "theirs"}
                options={[
                  { value: "theirs", label: t("draft.keepCurrent") },
                  { value: "mine", label: t("draft.keepMine") },
                ]}
                onChange={(value) => {
                  const next = new Set(kept);
                  if (value === "mine") next.add(index);
                  else next.delete(index);
                  setKept(next);
                }}
              />
            </>
          )}
        </RecordGroup>
      ))}
      <FormActions label={t("draft.reconcile.actions")}>
        <Button onClick={onDiscard}>{t("draft.discard")}</Button>
        <Button
          variant="primary"
          onClick={() => onRebase(rebase(head, results, kept, withCurrent))}
        >
          {t("draft.rebase")}
        </Button>
      </FormActions>
    </RecordGroup>
  );
}

export function RevisionNotices<T, Operation>({
  t,
  loading,
  viewed,
  head,
  draft,
  editing,
  pending,
  read,
  baseRead,
  reconcile,
  onOpen,
  onRetry,
}: {
  t: Translate;
  loading: string;
  viewed: string;
  head: string;
  draft: Draft<Operation> | undefined;
  editing: boolean;
  pending: string | undefined;
  read: ProjectionRead<T> | undefined;
  baseRead: ProjectionRead<T> | undefined;
  reconcile(base: T, current: T): ReactNode;
  onOpen(revision: string): void;
  onRetry(): void;
}) {
  return (
    <>
      {viewed !== head && !draft && (
        <Callout kind="note">{t("draft.historical")}</Callout>
      )}
      {editing && head !== viewed && (
        <Callout kind="warning">
          {t("draft.newer")}{" "}
          <Button onClick={() => onOpen(head)}>{t("draft.compare")}</Button>
        </Callout>
      )}
      {pending && draft && (
        <RecordGroup>
          {viewed !== head ? (
            <Callout kind="note">
              {t("draft.elsewhere")}{" "}
              <Button onClick={() => onOpen(pending)}>
                {t("draft.openDraft")}
              </Button>
            </Callout>
          ) : baseRead?.value && read?.value ? (
            reconcile(baseRead.value, read.value)
          ) : (
            <Metadata role="status">{loading}</Metadata>
          )}
        </RecordGroup>
      )}
      {(read?.error || baseRead?.error) && (
        <Callout kind="warning">
          {read?.error ?? baseRead?.error}{" "}
          <Button onClick={onRetry}>{t("draft.retry")}</Button>
        </Callout>
      )}
      {!read && <Metadata role="status">{loading}</Metadata>}
    </>
  );
}

export function DraftToolbar<Operation>({
  t,
  draft,
  saving,
  dirty,
  editable,
  onUndo,
  onDiscard,
  onSave,
}: {
  t: Translate;
  draft: Draft<Operation>;
  saving: boolean;
  dirty: boolean;
  editable: boolean;
  onUndo(): void;
  onDiscard(): void;
  onSave(): void;
}) {
  return (
    <Toolbar label={t("draft.title")}>
      <Metadata role="status">
        {saving
          ? t("draft.saving")
          : dirty
            ? t("draft.unsaved", { count: draft.operations.length })
            : t("draft.clean")}
      </Metadata>
      <Button disabled={!draft.undo.length || saving} onClick={onUndo}>
        {t("draft.undo")}
      </Button>
      <Button disabled={saving} onClick={onDiscard}>
        {t("draft.discard")}
      </Button>
      <Button variant="primary" disabled={!dirty || !editable} onClick={onSave}>
        {t("draft.save")}
      </Button>
    </Toolbar>
  );
}

type Handoff = "handed" | "stale" | "copied" | "failed";

// Hosts without a Composer collaborator still let the user carry the
// reference by hand.
export function useHandoff(state: LiveState, t: Translate) {
  const [handoff, setHandoff] = useState<Handoff>();
  const composer = state.context.composer;
  return {
    label: t(composer ? "handoff.add" : "handoff.copy"),
    clear: () => setHandoff(undefined),
    hand: (reference: ComposerReference) => {
      if (composer) {
        setHandoff(composer.addReference(reference) ? "handed" : "stale");
        return;
      }
      void copyText(reference.text).then((done) =>
        setHandoff(done ? "copied" : "failed"),
      );
    },
    status: handoff && (
      <Metadata role="status">{t(`handoff.${handoff}`)}</Metadata>
    ),
  };
}

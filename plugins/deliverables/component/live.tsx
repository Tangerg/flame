import {
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
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
import type { DocumentRead, Revision } from "./types";
import {
  DELIVERABLES_VIEW,
  encodeTarget,
  type Focus,
  type ViewTarget,
} from "./targets";
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
  | "history"
  | "open"
  | "operations"
  | "refresh"
  | "run"
  | "selection"
  | "selectRevision"
  | "signal"
  | "threads"
>;

export type ProjectionRead<T> = { value?: T; error?: string };

type ProjectionMethod = (typeof METHODS)["slides" | "sheets" | "blocks"];

// Revisions are read one at a time in the order wanted, so the shown revision
// comes before draft and comparison bases and the format queue stays short.
function useProjection<T>(
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

// An editor shows one revision with its projection. A newly read revision
// replaces it once its projection is read and no text entry is open, so
// following the head or saving keeps the editor's place.
function useShownRevision<T>(
  state: LiveState,
  incoming: DocumentRead,
  draft: { base: string; operations: unknown[] } | undefined,
  method: ProjectionMethod,
  schema: z.ZodType<T>,
  holding: boolean,
  comparedWith: (revision: Revision) => string | undefined,
) {
  const [shown, setShown] = useState(incoming);
  const viewed = shown.revision.id;
  const pending =
    draft && draft.base !== viewed && draft.operations.length
      ? draft.base
      : undefined;
  const compared = comparedWith(shown.revision);
  const projections = useProjection(
    state,
    shown.document.id,
    [
      ...new Set(
        [
          incoming.revision.id,
          viewed,
          pending,
          compared,
          comparedWith(incoming.revision),
        ].filter((revision) => revision !== undefined),
      ),
    ],
    method,
    schema,
  );
  if (incoming !== shown && !holding && projections.get(incoming.revision.id))
    setShown(incoming);
  const read = projections.get(viewed);
  const baseRead = pending ? projections.get(pending) : undefined;
  const comparedRead = compared ? projections.get(compared) : undefined;
  return {
    shown,
    pending,
    read,
    baseRead,
    compared,
    comparedRead,
    retry: () => {
      for (const [revision, entry] of [
        [viewed, read],
        [pending, baseRead],
        [compared, comparedRead],
      ] as const)
        if (revision && entry?.error) projections.retry(revision);
    },
  };
}

// The revisions before one, nearest first. History is paged newest first, so
// the chain ends where the loaded page does; the parent is always known.
function lineage(revision: Revision, history: readonly Revision[]) {
  const known = new Map(history.map((entry) => [entry.id, entry]));
  const chain: { id: string; revision?: Revision }[] = [];
  for (let id = revision.parent; id; id = known.get(id)?.parent)
    chain.push({ id, revision: known.get(id) });
  return chain;
}

// The editor follows the head while it shows the head, or a head it followed,
// and holds no unsaved draft. A revision the user opens that is not the head
// stays put.
function useFollowHead(
  state: LiveState,
  documentId: string,
  viewed: string,
  head: string,
  dirty: boolean,
) {
  const tracking = useRef<{
    viewed: string;
    target?: string;
    following: boolean;
  }>({ viewed, following: viewed === head });
  useLayoutEffect(() => {
    const before = tracking.current;
    const following =
      viewed === before.viewed || viewed === before.target
        ? before.following
        : viewed === head;
    const target = following && !dirty && head !== viewed ? head : undefined;
    tracking.current = { viewed, target, following };
    if (target) state.open({ id: documentId, revision: target });
  }, [viewed, head, dirty]);
}

// The draft records intent against the shown revision; edits apply only while
// it is the selected head, and saving re-checks it as the precondition.
export function useLiveEditor<T, Operation>(
  state: LiveState,
  {
    document,
    draft,
    onDraft,
    method,
    schema,
    holding,
  }: {
    document: DocumentRead;
    draft: Draft<Operation> | undefined;
    onDraft(update: DraftUpdate<Operation>): void;
    method: ProjectionMethod;
    schema: z.ZodType<T>;
    holding: boolean;
  },
) {
  // A chosen comparison base holds while it is older than the shown revision,
  // so following the head keeps showing everything since that revision.
  const [comparing, setComparing] = useState<{ open: boolean; base?: string }>({
    open: false,
  });
  const history = state.history?.value?.entries ?? [];
  const comparedWith = (revision: Revision) =>
    comparing.open
      ? (lineage(revision, history).find(({ id }) => id === comparing.base)
          ?.id ?? revision.parent)
      : undefined;
  const { shown, pending, read, baseRead, compared, comparedRead, retry } =
    useShownRevision(
      state,
      document,
      draft,
      method,
      schema,
      holding,
      comparedWith,
    );
  const documentId = shown.document.id;
  const head = shown.document.head;
  const viewed = shown.revision.id;
  const switching = state.selection?.revision !== viewed;
  const setDraft = (
    next: Draft<Operation> | undefined | DraftUpdate<Operation>,
  ) => onDraft(typeof next === "function" ? next : () => next);
  const [saving, setSaving] = useState(false);
  useEffect(() => {
    if (draft && draft.base !== viewed && !draft.operations.length)
      setDraft(undefined);
  }, [draft, viewed]);
  // A saved draft stays on screen until the editor shows the revision it
  // produced, so its edits do not vanish while that revision is read.
  const [settled, setSettled] = useState<Draft<Operation>>();
  if (settled && settled.base !== viewed) setSettled(undefined);
  const settling = settled?.base === viewed ? settled : undefined;
  const editedDraft = draft?.base === viewed ? draft : settling;
  const editable =
    viewed === head &&
    !switching &&
    !state.blocked &&
    !saving &&
    (!draft || editedDraft !== undefined);
  const dirty = !!draft?.operations.length;
  useFollowHead(state, documentId, viewed, head, dirty);

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
          setSettled(captured);
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
    document: shown,
    documentId,
    head,
    viewed,
    draft,
    setDraft,
    saving: saving || settling !== undefined,
    pending,
    switching,
    editedDraft,
    editable,
    dirty,
    read,
    baseRead,
    comparison: {
      open: comparing.open,
      setOpen: (open: boolean) =>
        setComparing((current) => ({ ...current, open })),
      base: compared,
      choices: lineage(shown.revision, history),
      choose: (base: string) => setComparing({ open: true, base }),
      read: comparedRead,
    },
    open: (revision: string) => state.open({ id: documentId, revision }),
    retry,
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
      return state.run(
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

export type LiveEditor<T, Operation> = ReturnType<
  typeof useLiveEditor<T, Operation>
>;

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
  live,
  reconcile,
}: {
  t: Translate;
  loading: string;
  live: LiveEditor<T, Operation>;
  reconcile(base: T, current: T): ReactNode;
}) {
  const { viewed, head, draft, pending, read, baseRead } = live;
  return (
    <>
      {viewed !== head && !draft && !live.switching && (
        <Callout kind="note">{t("draft.historical")}</Callout>
      )}
      {live.editedDraft && head !== viewed && (
        <Callout kind="warning">
          {t("draft.newer")}{" "}
          <Button onClick={() => live.open(head)}>{t("draft.compare")}</Button>
        </Callout>
      )}
      {pending && draft && (
        <RecordGroup>
          {viewed !== head ? (
            <Callout kind="note">
              {t("draft.elsewhere")}{" "}
              <Button onClick={() => live.open(pending)}>
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
          <Button onClick={live.retry}>{t("draft.retry")}</Button>
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

// Every selection handed to the Composer opens with the same identity lines so
// the model can address the exact document revision it was taken from.
export function selectionPreamble(document: DocumentRead): string[] {
  return [
    "Flame deliverable selection",
    `Document: ${JSON.stringify(document.document.title)} (${document.document.id})`,
    `Revision: ${document.revision.id}`,
  ];
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

// A link addresses the shown object. It pins the revision when the editor
// shows an earlier one or the object's identity is a position, so a link to
// the head otherwise keeps following it.
export function CopyLink({
  state,
  live,
  object,
  pinned = false,
  t,
}: {
  state: LiveState;
  live: { documentId: string; viewed: string; head: string };
  object?: string;
  pinned?: boolean;
  t: Translate;
}) {
  const [copied, setCopied] = useState<boolean>();
  const views = state.context.views;
  if (!views) return null;
  const target: ViewTarget = {
    document: live.documentId,
    ...(live.viewed === live.head && !pinned ? {} : { revision: live.viewed }),
    ...(object ? { object } : {}),
  };
  return (
    <>
      <Button
        onClick={() =>
          void copyText(
            views.href(DELIVERABLES_VIEW, encodeTarget(target)),
          ).then(setCopied)
        }
      >
        {t("link.copy")}
      </Button>
      {copied !== undefined && (
        <Metadata role="status">
          {t(copied ? "link.copied" : "link.failed")}
        </Metadata>
      )}
    </>
  );
}

// A link's object is located once the editor shows the revision it opened.
export function useFocus(
  focus: Focus | undefined,
  shown: string | undefined,
  locate: (object: string) => void,
) {
  const located = useRef<number>(undefined);
  useEffect(() => {
    if (!focus?.object || shown !== focus.revision) return;
    if (located.current === focus.key) return;
    located.current = focus.key;
    locate(focus.object);
  }, [focus, shown]);
}

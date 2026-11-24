import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from "react";
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
  PanelSection,
  RecordGroup,
  Toolbar,
  copyText,
} from "@flame/ui";
import {
  slideLayoutSchema,
  type ChartData,
  type ElementView,
  type Frame,
  type SlideView,
} from "../contracts/content";
import { METHODS } from "../contracts/methods";
import type { DocumentRead } from "./types";
import type { DeliverablesState } from "./useDeliverables";
import {
  operationsJSON,
  preview,
  record,
  revert,
  type Draft,
  type DraftOperation,
} from "./slides-draft";
import { type LayoutRead, label } from "./slides-presentation";
import { ChartFields, FrameFields, RunField } from "./slides-fields";
import {
  type Handoff,
  RevisionNotices,
  savedElement,
} from "./slides-revisions";
import { Canvas } from "./slides-canvas";

// The bounded format process admits one operation at a time, so revisions are
// read in sequence rather than in parallel.
function useLayouts(
  state: DeliverablesState,
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

function selectionReference(
  document: DocumentRead,
  slide: SlideView,
  element: ElementView | undefined,
  t: Translate,
): ComposerReference {
  const name = [
    document.document.title,
    t("slides.slide", { number: slide.number }),
    ...(element ? [label(element, t)] : []),
  ]
    .join(" · ")
    .replace(/\s+/gu, " ")
    .trim();
  const text = [
    "Flame deliverable selection",
    `Document: ${JSON.stringify(document.document.title)} (${document.document.id})`,
    `Revision: ${document.revision.id}`,
    `Slide ${slide.number}${slide.name ? ` ${JSON.stringify(slide.name)}` : ""}: ${slide.id}`,
    ...(element
      ? [
          `Element ${JSON.stringify(element.name)} (${element.kind}): ${element.id}`,
        ]
      : []),
    "Read this exact revision with the Deliverables slides tool and carry its current values as expect when editing.",
  ].join("\n");
  return { label: name, text };
}

function editableTarget(event: KeyboardEvent): boolean {
  const target = event.target as HTMLElement;
  return (
    target.isContentEditable ||
    ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName)
  );
}

export type DraftUpdate = (current: Draft | undefined) => Draft | undefined;

// The draft records intent against the revision it was read from; edits apply only
// while that revision is the head, and saving re-checks it as the precondition.
function useSlideDraft(
  state: DeliverablesState,
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

function ElementInspector({
  t,
  element,
  original,
  editable,
  revertible,
  dirty,
  reference,
  onFrame,
  onText,
  onChart,
  onDelete,
  onRevert,
}: {
  t: Translate;
  element: ElementView;
  original: ElementView | undefined;
  editable: boolean;
  revertible: boolean;
  dirty: boolean;
  reference: ReactNode;
  onFrame(frame: Frame): void;
  onText(id: string, expect: string, text: string): void;
  onChart(expect: ChartData, data: ChartData): void;
  onDelete(): void;
  onRevert(): void;
}) {
  return (
    <RecordGroup>
      <Facts
        values={[
          [t("slides.element"), t(`slides.kind.${element.kind}`)],
          [t("slides.name"), element.name || null],
        ]}
      />
      {element.frameSource && element.frameSource !== "slide" && (
        <Metadata>{t(`slides.inherited.${element.frameSource}`)}</Metadata>
      )}
      {element.frame && (
        <FrameFields
          frame={element.frame}
          editable={editable}
          onCommit={onFrame}
          t={t}
        />
      )}
      {element.text.map((run, index) => {
        const saved =
          original?.text.find((entry) => entry.id === run.id)?.text ?? run.text;
        return (
          <RunField
            key={run.id}
            label={t("slides.run", { number: index + 1 })}
            text={run.text}
            editable={editable}
            onCommit={(text) => onText(run.id, saved, text)}
          />
        );
      })}
      {element.kind === "chart" && !element.chart && (
        <Metadata>{t("slides.chart.unsupported")}</Metadata>
      )}
      {element.chart && original?.chart && (
        <ChartFields
          chart={element.chart}
          editable={editable}
          onCommit={(data) => onChart(original.chart!.data, data)}
          t={t}
        />
      )}
      <Toolbar label={t("slides.elementActions")}>
        <Button disabled={!editable} onClick={onDelete}>
          {t("slides.delete")}
        </Button>
        <Button disabled={!revertible} onClick={onRevert}>
          {t("slides.revert")}
        </Button>
        {reference}
      </Toolbar>
      {dirty && <Metadata>{t("slides.reference.unsaved")}</Metadata>}
    </RecordGroup>
  );
}

function SlideInspector({
  t,
  slide,
  saved,
  slideCount,
  editable,
  dirty,
  reference,
  onText,
  onMove,
  onRestructure,
}: {
  t: Translate;
  slide: SlideView;
  saved: SlideView | undefined;
  slideCount: number;
  editable: boolean;
  dirty: boolean;
  reference: ReactNode;
  onText(id: string, expect: string, text: string): void;
  onMove(offset: number): void;
  onRestructure(operations: unknown[]): void;
}) {
  const [confirmingRemoval, setConfirmingRemoval] = useState(false);
  const structural = editable && !dirty;
  return (
    <RecordGroup>
      <Metadata>{t("slides.select")}</Metadata>
      {slide.notes.map((run, index) => {
        const savedText =
          saved?.notes.find((entry) => entry.id === run.id)?.text ?? run.text;
        return (
          <RunField
            key={run.id}
            label={t("slides.notes", { number: index + 1 })}
            text={run.text}
            editable={editable}
            onCommit={(text) => onText(run.id, savedText, text)}
          />
        );
      })}
      <Toolbar label={t("slides.slideActions")}>
        <Button
          disabled={!structural || slide.number === 1}
          onClick={() => onMove(-1)}
        >
          {t("slides.moveUp")}
        </Button>
        <Button
          disabled={!structural || slide.number === slideCount}
          onClick={() => onMove(1)}
        >
          {t("slides.moveDown")}
        </Button>
        <Button
          disabled={!structural}
          onClick={() =>
            onRestructure([{ type: "duplicateSlide", id: slide.id }])
          }
        >
          {t("slides.duplicate")}
        </Button>
        <Button
          disabled={!structural || slideCount < 2}
          onClick={() => setConfirmingRemoval(true)}
        >
          {t("slides.deleteSlide")}
        </Button>
        {reference}
      </Toolbar>
      {dirty && <Metadata>{t("slides.structureBlocked")}</Metadata>}
      {confirmingRemoval && (
        <RecordGroup>
          <Callout kind="warning">
            {t("slides.deleteSlide.confirm", { number: slide.number })}
          </Callout>
          <FormActions label={t("slides.slideActions")}>
            <Button onClick={() => setConfirmingRemoval(false)}>
              {t("cancel")}
            </Button>
            <Button
              disabled={!structural}
              onClick={() => {
                setConfirmingRemoval(false);
                onRestructure([{ type: "deleteSlide", id: slide.id }]);
              }}
            >
              {t("slides.deleteSlide")}
            </Button>
          </FormActions>
        </RecordGroup>
      )}
    </RecordGroup>
  );
}

function DraftToolbar({
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
  draft: Draft;
  saving: boolean;
  dirty: boolean;
  editable: boolean;
  onUndo(): void;
  onDiscard(): void;
  onSave(): void;
}) {
  return (
    <Toolbar label={t("slides.draft")}>
      <Metadata role="status">
        {saving
          ? t("slides.saving")
          : dirty
            ? t("slides.unsaved", { count: draft.operations.length })
            : t("slides.clean")}
      </Metadata>
      <Button disabled={!draft.undo.length || saving} onClick={onUndo}>
        {t("slides.undo")}
      </Button>
      <Button disabled={saving} onClick={onDiscard}>
        {t("slides.discard")}
      </Button>
      <Button variant="primary" disabled={!dirty || !editable} onClick={onSave}>
        {t("slides.save")}
      </Button>
    </Toolbar>
  );
}

// The workspace owns drafts by document, so reading another revision (which
// unmounts this view while it loads) never discards unsubmitted intent.
export function SlideStudio({
  state,
  document,
  draft,
  onDraft,
  t,
}: {
  state: DeliverablesState;
  document: DocumentRead;
  draft: Draft | undefined;
  onDraft(update: DraftUpdate): void;
  t: Translate;
}) {
  const {
    documentId,
    head,
    viewed,
    setDraft,
    saving,
    pending,
    editedDraft,
    editable,
    dirty,
    change,
    undo,
    save,
    restructure,
  } = useSlideDraft(state, document, draft, onDraft);
  const [slideId, setSlideId] = useState<string>();
  const [elementId, setElementId] = useState<string>();
  const [handoff, setHandoff] = useState<Handoff>();
  const composer = state.context.composer;
  // Hosts without a Composer collaborator still let the user carry the
  // reference by hand.
  const hand = (reference: ComposerReference) => {
    if (composer) {
      setHandoff(composer.addReference(reference) ? "handed" : "stale");
      return;
    }
    void copyText(reference.text).then((done) =>
      setHandoff(done ? "copied" : "failed"),
    );
  };
  const layouts = useLayouts(
    state,
    documentId,
    pending ? [viewed, pending] : [viewed],
  );
  const read = layouts.get(viewed);
  const baseRead = pending ? layouts.get(pending) : undefined;
  const savedLayout = read?.value;
  const layout = useMemo(
    () =>
      savedLayout && editedDraft
        ? preview(savedLayout, editedDraft.operations)
        : savedLayout,
    [savedLayout, editedDraft],
  );
  const slide =
    layout?.slides.find((entry) => entry.id === slideId) ?? layout?.slides[0];
  const element = slide?.elements.find((entry) => entry.id === elementId);
  const original = savedElement(savedLayout, elementId);
  const savedSlide = savedLayout?.slides.find(
    (entry) => entry.id === slide?.id,
  );
  const changed = (target: ElementView) =>
    !!draft?.operations.some(
      (operation) =>
        operation.id === target.id ||
        target.text.some((run) => run.id === operation.id),
    );

  const setFrame = (target: ElementView, frame: Frame) => {
    const source = savedElement(savedLayout, target.id)?.frame;
    if (source)
      change({ type: "setFrame", id: target.id, expect: source, frame });
  };
  const setText = (id: string, expect: string, text: string) =>
    change({ type: "replaceText", id, expect, text });
  const remove = (target: ElementView) => {
    change({ type: "deleteElement", id: target.id }, target);
    setElementId(undefined);
  };
  const move = (offset: number) => {
    if (!layout || !slide) return;
    const ids = layout.slides.map((entry) => entry.id);
    const index = ids.indexOf(slide.id);
    const target = index + offset;
    if (target < 0 || target >= ids.length) return;
    [ids[index], ids[target]] = [ids[target]!, ids[index]!];
    restructure([{ type: "reorderSlides", ids }]);
  };
  // A reference always names the saved revision, never unsaved draft values.
  const reference = (
    <Button
      disabled={dirty || !savedSlide || (element !== undefined && !original)}
      onClick={() => {
        if (savedSlide)
          hand(selectionReference(document, savedSlide, original, t));
      }}
    >
      {t(composer ? "slides.handoff" : "slides.reference")}
    </Button>
  );

  return (
    <PanelSection title={t("slides.title")} description={t("slides.help")}>
      <RevisionNotices
        t={t}
        viewed={viewed}
        head={head}
        draft={draft}
        editing={editedDraft !== undefined}
        pending={pending}
        read={read}
        baseRead={baseRead}
        onOpen={(revision) => state.open({ id: documentId, revision })}
        onRebase={setDraft}
        onDiscard={() => setDraft(undefined)}
        onRetry={() => {
          if (read?.error) layouts.retry(viewed);
          if (pending && baseRead?.error) layouts.retry(pending);
        }}
      />
      {layout && slide && (
        <div
          onKeyDown={(event) => {
            if (
              (event.metaKey || event.ctrlKey) &&
              event.key.toLowerCase() === "z" &&
              !editableTarget(event)
            ) {
              event.preventDefault();
              undo();
            }
          }}
          style={{
            display: "flex",
            flexWrap: "wrap",
            gap: "var(--layout-content-gap, 12px)",
            alignItems: "flex-start",
          }}
        >
          <nav
            aria-label={t("slides.list")}
            style={{ flex: "0 1 11rem", minWidth: "9rem" }}
          >
            <RecordGroup variant="list">
              {layout.slides.map((entry) => (
                <Button
                  key={entry.id}
                  aria-current={entry.id === slide.id ? "page" : undefined}
                  active={entry.id === slide.id}
                  onClick={() => {
                    setSlideId(entry.id);
                    setElementId(undefined);
                  }}
                >
                  {t("slides.slide", { number: entry.number })}
                  {entry.name ? ` · ${entry.name}` : ""}
                  {entry.hidden && <Badge>{t("slides.hidden")}</Badge>}
                </Button>
              ))}
            </RecordGroup>
          </nav>
          <div style={{ flex: "3 1 22rem", minWidth: 0 }}>
            <Canvas
              layout={layout}
              slide={slide}
              selected={element?.id}
              editable={editable}
              onSelect={(id) => {
                setElementId(id);
                setHandoff(undefined);
              }}
              onFrame={setFrame}
              onDelete={remove}
              t={t}
            />
            <RecordGroup variant="list" aria-label={t("slides.elements")}>
              {slide.elements.map((entry) => (
                <Button
                  key={entry.id}
                  active={entry.id === element?.id}
                  aria-pressed={entry.id === element?.id}
                  onClick={() => {
                    setElementId(entry.id);
                    setHandoff(undefined);
                  }}
                >
                  {t(`slides.kind.${entry.kind}`)} · {label(entry, t)}
                  {!entry.frame && ` · ${t("slides.noFrame")}`}
                  {changed(entry) && <Badge>{t("slides.changed")}</Badge>}
                </Button>
              ))}
            </RecordGroup>
          </div>
          <div style={{ flex: "2 1 16rem", minWidth: 0 }}>
            {element ? (
              <ElementInspector
                t={t}
                element={element}
                original={original}
                editable={editable}
                revertible={!!editedDraft && changed(element) && !!original}
                dirty={dirty}
                reference={reference}
                onFrame={(frame) => setFrame(element, frame)}
                onText={setText}
                onChart={(expect, data) =>
                  change({ type: "setChartData", id: element.id, expect, data })
                }
                onDelete={() => remove(element)}
                onRevert={() => {
                  if (original)
                    setDraft((current) => current && revert(current, original));
                }}
              />
            ) : (
              <SlideInspector
                key={slide.id}
                t={t}
                slide={slide}
                saved={savedSlide}
                slideCount={layout.slides.length}
                editable={editable}
                dirty={dirty}
                reference={reference}
                onText={setText}
                onMove={move}
                onRestructure={restructure}
              />
            )}
            {handoff && (
              <Metadata role="status">
                {t(`slides.reference.${handoff}`)}
              </Metadata>
            )}
          </div>
        </div>
      )}
      {editedDraft && (
        <DraftToolbar
          t={t}
          draft={editedDraft}
          saving={saving}
          dirty={dirty}
          editable={editable}
          onUndo={undo}
          onDiscard={() => setDraft(undefined)}
          onSave={() => void save()}
        />
      )}
    </PanelSection>
  );
}

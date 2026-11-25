import { useMemo, useState } from "react";
import { type ComposerReference, type Translate } from "@flame/component-sdk";
import {
  Badge,
  Button,
  Metadata,
  PanelSection,
  RecordGroup,
  Toolbar,
  copyText,
} from "@flame/ui";
import { type ElementView, type Frame } from "../contracts/content";
import * as token from "@flame/ui/tokens.stylex";
import type { DocumentRead } from "./types";
import { preview, revert, type Draft } from "./slides-draft";
import { label } from "./slides-presentation";
import {
  type Handoff,
  RevisionNotices,
  savedElement,
} from "./slides-revisions";
import { Canvas } from "./slides-canvas";
import {
  type DraftUpdate,
  type SlideStudioState,
  useLayouts,
  useSlideDraft,
} from "./useSlideDraft";
import {
  ElementInspector,
  SlideInspector,
  editableTarget,
  selectionReference,
} from "./slides-inspector";

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
  state: SlideStudioState;
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
            gap: token.layout.contentGap,
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

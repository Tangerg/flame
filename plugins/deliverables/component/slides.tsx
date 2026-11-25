import { useMemo, useRef, useState } from "react";
import { type Translate } from "@flame/component-sdk";
import { Badge, Button, PanelSection, RecordGroup } from "@flame/ui";
import { type Frame } from "../contracts/content";
import {
  slideLayoutSchema,
  type ElementView,
  type SlideLayout,
  type SlideView,
} from "../contracts/layouts";
import { METHODS } from "../contracts/methods";
import * as token from "@flame/ui/tokens.stylex";
import type { DocumentRead } from "./types";
import type { Draft, DraftUpdate } from "./live-draft";
import {
  CopyLink,
  DraftToolbar,
  RevisionNotices,
  useFocus,
  useHandoff,
  useLiveEditor,
  type LiveEditor,
  type LiveState,
} from "./live";
import { preview, record, revert, type SlideOperation } from "./slides-draft";
import {
  describeSlideObject,
  label,
  slideComment,
  slideTarget,
} from "./slides-presentation";
import { Comments } from "./comments";
import { SlideReconcile, savedElement } from "./slides-revisions";
import { RevisionChanges, SlideChanges } from "./changes";
import { Canvas } from "./slides-canvas";
import { SlideAdditions, useInsertion } from "./slides-additions";
import type { Focus } from "./targets";
import {
  ElementInspector,
  SlideInspector,
  editableTarget,
  selectionReference,
} from "./slides-inspector";

// A reference always names the saved revision, never unsaved draft values.
function SelectionActions({
  state,
  live,
  handoff,
  saved,
  element,
  original,
  t,
}: {
  state: LiveState;
  live: LiveEditor<SlideLayout, SlideOperation>;
  handoff: ReturnType<typeof useHandoff>;
  saved: SlideView | undefined;
  element: ElementView | undefined;
  original: ElementView | undefined;
  t: Translate;
}) {
  return (
    <>
      <Button
        disabled={live.dirty || !saved || (element !== undefined && !original)}
        onClick={() => {
          if (saved)
            handoff.hand(selectionReference(live.document, saved, original, t));
        }}
      >
        {handoff.label}
      </Button>
      <CopyLink
        state={state}
        live={live}
        object={element?.id ?? saved?.id}
        t={t}
      />
    </>
  );
}

export function SlideStudio({
  state,
  document,
  draft,
  onDraft,
  link,
  dateTime,
  t,
}: {
  state: LiveState;
  document: DocumentRead;
  draft: Draft<SlideOperation> | undefined;
  onDraft(update: DraftUpdate<SlideOperation>): void;
  link?: Focus;
  dateTime: Intl.DateTimeFormat;
  t: Translate;
}) {
  // Slide and element identities survive revisions, so an open field carries
  // over to the next revision.
  const live = useLiveEditor(state, {
    document,
    draft,
    onDraft,
    method: METHODS.slides,
    schema: slideLayoutSchema,
    holding: false,
  });
  const {
    head,
    setDraft,
    saving,
    editedDraft,
    editable,
    dirty,
    apply,
    undo,
    restructure,
  } = live;
  const change = (operation: SlideOperation, target?: ElementView) =>
    apply((current) => record(current, operation, target));
  const [slideId, setSlideId] = useState<string>();
  const [elementId, setElementId] = useState<string>();
  const handoff = useHandoff(state, t);
  const editor = useRef<HTMLDivElement>(null);
  const savedLayout = live.read?.value;
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
  const insert = useInsertion(live, savedLayout, (slide, added) => {
    setSlideId(slide);
    setElementId(added);
  });
  const additions = savedSlide && savedLayout && (
    <SlideAdditions
      layout={savedLayout}
      slide={savedSlide}
      disabled={!editable || dirty}
      onInsert={insert}
      t={t}
    />
  );
  const move = (offset: number) => {
    if (!layout || !slide) return;
    const ids = layout.slides.map((entry) => entry.id);
    const index = ids.indexOf(slide.id);
    const target = index + offset;
    if (target < 0 || target >= ids.length) return;
    [ids[index], ids[target]] = [ids[target]!, ids[index]!];
    restructure([{ type: "reorderSlides", ids }]);
  };
  const locate = (object: string) => {
    const target = slideTarget(object);
    setSlideId(target.slide);
    setElementId(target.element);
    handoff.clear();
    editor.current?.scrollIntoView({ block: "nearest" });
  };
  useFocus(link, savedLayout && live.viewed, locate);
  const reference = (
    <SelectionActions
      state={state}
      live={live}
      handoff={handoff}
      saved={savedSlide}
      element={element}
      original={original}
      t={t}
    />
  );

  return (
    <PanelSection title={t("slides.title")} description={t("slides.help")}>
      <RevisionNotices
        t={t}
        loading={t("slides.loading")}
        live={live}
        reconcile={(base, current) =>
          draft && (
            <SlideReconcile
              base={base}
              head={{ id: head, layout: current }}
              draft={draft}
              onRebase={setDraft}
              onDiscard={() => setDraft(undefined)}
              t={t}
            />
          )
        }
      />
      <RevisionChanges live={live} dateTime={dateTime} t={t}>
        {(before, after) => (
          <SlideChanges
            before={before}
            after={after}
            onOpen={(target) => locate(target.element ?? target.slide)}
            t={t}
          />
        )}
      </RevisionChanges>
      {savedLayout && savedSlide && (
        <Comments
          state={state}
          document={live.document}
          selection={slideComment(savedSlide, element?.id, t)}
          describe={(anchor) =>
            describeSlideObject(savedLayout, anchor.object!, t)
          }
          onLocate={locate}
          handoff={handoff}
          dateTime={dateTime}
          t={t}
        />
      )}
      {layout && slide && (
        <div
          ref={editor}
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
                handoff.clear();
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
                    handoff.clear();
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
                additions={additions}
                onText={setText}
                onMove={move}
                onRestructure={restructure}
              />
            )}
            {handoff.status}
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
          onSave={() => void live.save()}
        />
      )}
    </PanelSection>
  );
}

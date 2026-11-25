import { useMemo, useRef, useState } from "react";
import type { ComposerReference, Translate } from "@flame/component-sdk";
import { Button, Metadata, PanelSection } from "@flame/ui";
import {
  documentLayoutSchema,
  type BlockView,
  type DocumentLayout,
} from "../contracts/layouts";
import { METHODS } from "../contracts/methods";
import type { DocumentRead } from "./types";
import type { Draft, DraftUpdate } from "./live-draft";
import {
  CopyLink,
  DraftToolbar,
  Reconcile,
  RevisionNotices,
  selectionPreamble,
  useFocus,
  useHandoff,
  useLiveEditor,
  type LiveState,
} from "./live";
import type { Focus } from "./targets";
import {
  blockText,
  insert,
  preview,
  reconcile,
  styleOf,
  withCurrent,
  type DocItem,
  type DocOperation,
} from "./docs-draft";
import { InsertedBlock, SavedBlock, type Selection } from "./docs-blocks";
import { BlockChanges, RevisionChanges, blockTitle } from "./changes";
import { Comments, quoteOf, type Anchor } from "./comments";
import { Suggestions } from "./suggestions";
import { editableTarget } from "./slides-inspector";

function blockReference(
  document: DocumentRead,
  block: BlockView,
): ComposerReference {
  const excerpt = blockText(block).replace(/\s+/gu, " ").trim();
  return {
    label: [document.document.title, excerpt.slice(0, 40) || block.kind]
      .join(" · ")
      .trim(),
    text: [
      ...selectionPreamble(document),
      `Block ${block.id} (${block.kind}${block.kind === "paragraph" ? `, style ${JSON.stringify(styleOf(block.style))}` : ""}), expect ${block.expect}`,
      ...(block.kind === "paragraph"
        ? [
            "Text runs:",
            ...block.runs.map(
              (run) => `${run.id}\t${JSON.stringify(run.text)}`,
            ),
          ]
        : [`Text: ${JSON.stringify(blockText(block))}`]),
      "Read this exact revision with the Deliverables blocks tool; carry the block expect for block edits and each run's text as its replaceText expect.",
    ].join("\n"),
  };
}

export function describeDocOperation(operation: DocOperation, t: Translate) {
  switch (operation.type) {
    case "replaceText":
      return t("docs.op.replaceText", { text: operation.text.slice(0, 60) });
    case "insertParagraph":
      return t("docs.op.insertParagraph", {
        text: operation.text.slice(0, 60),
      });
    case "setParagraphStyle":
      return t("docs.op.setParagraphStyle", {
        style: t(`docs.style.${operation.style}`),
      });
    case "deleteBlock":
      return t("docs.op.deleteBlock");
  }
}

function DocReconcile({
  head,
  layout,
  draft,
  onRebase,
  onDiscard,
  t,
}: {
  head: string;
  layout: DocumentLayout;
  draft: Draft<DocOperation>;
  onRebase(draft: Draft<DocOperation>): void;
  onDiscard(): void;
  t: Translate;
}) {
  const results = useMemo(
    () => reconcile(layout, draft.operations),
    [layout, draft.operations],
  );
  const describe = (operation: DocOperation) =>
    describeDocOperation(operation, t);
  return (
    <Reconcile
      results={results}
      head={head}
      describe={describe}
      mine={(operation) =>
        operation.type === "replaceText" ? operation.text : describe(operation)
      }
      theirs={(result) =>
        typeof result.current === "string"
          ? result.current
          : (result.current as { text: string }).text
      }
      withCurrent={withCurrent}
      onRebase={onRebase}
      onDiscard={onDiscard}
      t={t}
    />
  );
}

// A block is named by its text, or by its kind when it has none.
function blockLabel(block: BlockView, t: Translate): string {
  const text = blockText(block);
  if (!text) return blockTitle(block, t);
  return text.length > 40 ? `“${text.slice(0, 40)}…”` : `“${text}”`;
}

function blockComment(
  layout: DocumentLayout | undefined,
  selection: Selection | undefined,
  t: Translate,
) {
  const block =
    selection?.source === "saved"
      ? layout?.blocks.find((entry) => entry.id === selection.id)
      : undefined;
  return (
    block && {
      object: block.id,
      quote: blockText(block),
      label: blockLabel(block, t),
    }
  );
}

// Block identities are positions, so on another revision the block at the
// anchored position is the same one only while its text still matches.
function describeBlock(
  layout: DocumentLayout,
  viewed: string,
  anchor: Anchor,
  t: Translate,
): string | undefined {
  const block = layout.blocks.find((entry) => entry.id === anchor.object);
  if (!block) return undefined;
  if (
    anchor.revision !== viewed &&
    quoteOf(blockText(block)) !== (anchor.quote ?? "")
  )
    return undefined;
  return blockLabel(block, t);
}

export function DocumentStudio({
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
  draft: Draft<DocOperation> | undefined;
  onDraft(update: DraftUpdate<DocOperation>): void;
  link?: Focus;
  dateTime: Intl.DateTimeFormat;
  t: Translate;
}) {
  // Block identities are positions in one revision, so a selection ends when
  // the editor shows another revision.
  const [focus, setFocus] = useState<{
    revision: string;
    target: Selection;
    editing: boolean;
  }>();
  const live = useLiveEditor(state, {
    document,
    draft,
    onDraft,
    method: METHODS.blocks,
    schema: documentLayoutSchema,
    holding: !!focus?.editing,
  });
  const handoff = useHandoff(state, t);
  const body = useRef<HTMLDivElement>(null);
  const layout = live.read?.value;
  const current = focus?.revision === live.viewed ? focus : undefined;
  const items = useMemo(
    () => (layout ? preview(layout, live.editedDraft?.operations ?? []) : []),
    [layout, live.editedDraft],
  );
  const select = (target: Selection | undefined, editing = false) => {
    setFocus(target && { revision: live.viewed, target, editing });
    handoff.clear();
  };
  const insertAfter = (after: BlockView | undefined) => {
    if (!live.editable) return;
    const index = live.editedDraft?.operations.length ?? 0;
    live.apply((draft) => insert(draft, after));
    select({ source: "inserted", index }, true);
  };
  const locate = (id: string) => {
    select({ source: "saved", id });
    body.current
      ?.querySelector(`[data-block="${CSS.escape(id)}"]`)
      ?.scrollIntoView({ block: "center" });
  };
  useFocus(link, layout && live.viewed, locate);
  const isSelected = (item: DocItem) => {
    const target = current?.target;
    return item.source === "saved"
      ? target?.source === "saved" && target.id === item.block.id
      : target?.source === "inserted" && target.index === item.index;
  };

  return (
    <PanelSection title={t("docs.title")} description={t("docs.help")}>
      <RevisionNotices
        t={t}
        loading={t("docs.loading")}
        live={live}
        reconcile={(_base, current) =>
          draft && (
            <DocReconcile
              head={live.head}
              layout={current}
              draft={draft}
              onRebase={live.setDraft}
              onDiscard={() => live.setDraft(undefined)}
              t={t}
            />
          )
        }
      />
      <Suggestions
        state={state}
        live={live}
        describe={(operation) => describeDocOperation(operation, t)}
        dateTime={dateTime}
        t={t}
      />
      <RevisionChanges live={live} dateTime={dateTime} t={t}>
        {(before, after) => (
          <BlockChanges before={before} after={after} onOpen={locate} t={t} />
        )}
      </RevisionChanges>
      {layout && (
        <Comments
          state={state}
          document={live.document}
          selection={blockComment(layout, current?.target, t)}
          describe={(anchor) => describeBlock(layout, live.viewed, anchor, t)}
          onLocate={locate}
          handoff={handoff}
          dateTime={dateTime}
          t={t}
        />
      )}
      {layout && (
        <div
          onKeyDown={(event) => {
            if (
              (event.metaKey || event.ctrlKey) &&
              event.key.toLowerCase() === "z" &&
              !editableTarget(event)
            ) {
              event.preventDefault();
              live.undo();
            }
          }}
        >
          <Button
            disabled={!live.editable}
            onClick={() => insertAfter(undefined)}
          >
            {t("docs.insertStart")}
          </Button>
          <div
            ref={body}
            role="list"
            aria-label={t("docs.body")}
            style={{
              display: "grid",
              gap: 4,
              padding: "16px 12px",
              marginBlockStart: 8,
              border: "1px solid var(--color-border)",
              borderRadius: "var(--shape-sm)",
              background: "var(--color-surface)",
            }}
          >
            {items.map((item) =>
              item.source === "saved" ? (
                <SavedBlock
                  key={item.block.id}
                  item={item}
                  selected={isSelected(item)}
                  editing={isSelected(item) && !!current?.editing}
                  editable={live.editable}
                  dirty={live.dirty}
                  apply={live.apply}
                  onSelect={select}
                  onInsert={insertAfter}
                  onHand={(block) =>
                    handoff.hand(blockReference(live.document, block))
                  }
                  handLabel={handoff.label}
                  t={t}
                />
              ) : (
                <InsertedBlock
                  key={`inserted:${item.index}`}
                  index={item.index}
                  paragraph={item.paragraph}
                  selected={isSelected(item)}
                  editing={isSelected(item) && !!current?.editing}
                  editable={live.editable}
                  apply={live.apply}
                  onSelect={select}
                  t={t}
                />
              ),
            )}
          </div>
          <Metadata>{t("docs.keyboard")}</Metadata>
          {live.dirty && <Metadata>{t("handoff.unsaved")}</Metadata>}
          {handoff.status}
          <CopyLink
            state={state}
            live={live}
            object={
              current?.target.source === "saved" ? current.target.id : undefined
            }
            pinned
            t={t}
          />
        </div>
      )}
      {live.editedDraft && (
        <DraftToolbar
          t={t}
          draft={live.editedDraft}
          saving={live.saving}
          dirty={live.dirty}
          editable={live.editable}
          onUndo={live.undo}
          onDiscard={() => live.setDraft(undefined)}
          onSave={() => void live.save()}
        />
      )}
    </PanelSection>
  );
}

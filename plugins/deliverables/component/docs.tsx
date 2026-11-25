import { useMemo, useState } from "react";
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
  DraftToolbar,
  Reconcile,
  RevisionNotices,
  useHandoff,
  useLiveEditor,
  type LiveState,
} from "./live";
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
      "Flame deliverable selection",
      `Document: ${JSON.stringify(document.document.title)} (${document.document.id})`,
      `Revision: ${document.revision.id}`,
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
  const describe = (operation: DocOperation) => {
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
  };
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

export function DocumentStudio({
  state,
  document,
  draft,
  onDraft,
  t,
}: {
  state: LiveState;
  document: DocumentRead;
  draft: Draft<DocOperation> | undefined;
  onDraft(update: DraftUpdate<DocOperation>): void;
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

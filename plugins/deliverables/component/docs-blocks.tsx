import { useRef, useState, type CSSProperties } from "react";
import type { Translate } from "@flame/component-sdk";
import { Badge, Button, Metadata, Segmented, Toolbar } from "@flame/ui";
import type { BlockView, ParagraphView } from "../contracts/layouts";
import type { Draft } from "./live-draft";
import {
  PARAGRAPH_STYLES,
  changeInserted,
  editText,
  remove,
  restore,
  setStyle,
  styleOf,
  textOf,
  type DocItem,
  type DocOperation,
  type InsertedParagraph,
  type ParagraphStyle,
} from "./docs-draft";

export type Selection =
  { source: "saved"; id: string } | { source: "inserted"; index: number };

const HEADING_SIZES: Record<string, string> = {
  Title: "1.75em",
  Heading1: "1.5em",
  Heading2: "1.25em",
  Heading3: "1.1em",
};

function paragraphStyle(style: string, alignment?: string): CSSProperties {
  const size = HEADING_SIZES[style];
  return {
    margin: 0,
    fontSize: size,
    fontWeight: size ? 600 : undefined,
    lineHeight: 1.5,
    textAlign:
      alignment === "center"
        ? "center"
        : alignment === "right" || alignment === "end"
          ? "end"
          : alignment === "both"
            ? "justify"
            : undefined,
    whiteSpace: "pre-wrap",
    overflowWrap: "anywhere",
  };
}

// A paragraph has no line breaks here, so Enter writes it into the draft.
function ParagraphEditor({
  text,
  style,
  alignment,
  label,
  onCommit,
  onCancel,
}: {
  text: string;
  style: string;
  alignment?: string;
  label: string;
  onCommit(text: string): void;
  onCancel(): void;
}) {
  const [value, setValue] = useState(text);
  // Enter, blur and Escape can each end the entry; only the first counts.
  const settled = useRef(false);
  const settle = (commit: boolean) => {
    if (settled.current) return;
    settled.current = true;
    if (commit) onCommit(value);
    else onCancel();
  };
  return (
    <textarea
      aria-label={label}
      autoFocus
      value={value}
      rows={Math.max(1, Math.ceil(value.length / 80))}
      onChange={(event) => setValue(event.target.value.replace(/\n/gu, ""))}
      onBlur={() => settle(true)}
      onKeyDown={(event) => {
        if (event.key === "Enter") {
          event.preventDefault();
          settle(true);
        } else if (event.key === "Escape") {
          event.preventDefault();
          settle(false);
        }
      }}
      style={{
        ...paragraphStyle(style, alignment),
        display: "block",
        width: "100%",
        boxSizing: "border-box",
        resize: "none",
        border: 0,
        outline: "none",
        padding: 0,
        background: "transparent",
        color: "inherit",
        font: "inherit",
      }}
    />
  );
}

function ParagraphText({ paragraph }: { paragraph: ParagraphView }) {
  if (!paragraph.runs.some((run) => run.text)) return null;
  return (
    <>
      {paragraph.runs.map((run) => (
        <span
          key={run.id}
          style={{
            fontWeight: run.bold ? 700 : undefined,
            fontStyle: run.italic ? "italic" : undefined,
            textDecoration: run.underline ? "underline" : undefined,
          }}
        >
          {run.text}
        </span>
      ))}
    </>
  );
}

function BlockContent({ block, t }: { block: BlockView; t: Translate }) {
  if (block.kind === "table")
    return (
      <table
        aria-label={t("docs.table")}
        style={{ borderCollapse: "collapse", width: "100%" }}
      >
        <tbody>
          {block.rows.map((row, index) => (
            <tr key={index}>
              {row.map((cell, column) => (
                <td
                  key={column}
                  style={{
                    border: "1px solid var(--color-border)",
                    padding: "4px 8px",
                    verticalAlign: "top",
                    whiteSpace: "pre-wrap",
                  }}
                >
                  {cell}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    );
  if (block.kind === "other")
    return (
      <Metadata>
        {t("docs.other", { element: block.element })}
        {block.text ? ` · ${block.text}` : ""}
      </Metadata>
    );
  const empty = !textOf(block.runs);
  return (
    <p style={paragraphStyle(block.style, block.alignment)}>
      {block.list && "• "}
      {empty ? (
        <span style={{ color: "var(--color-text-soft)" }}>
          {t("docs.empty")}
        </span>
      ) : (
        <ParagraphText paragraph={block} />
      )}
    </p>
  );
}

function StyleChoice({
  value,
  disabled,
  onChange,
  t,
}: {
  value: string;
  disabled: boolean;
  onChange(style: ParagraphStyle): void;
  t: Translate;
}) {
  return (
    <Segmented<ParagraphStyle>
      ariaLabel={t("docs.style")}
      value={styleOf(value) as ParagraphStyle}
      disabled={disabled}
      options={PARAGRAPH_STYLES.map((style) => ({
        value: style,
        label: t(`docs.style.${style}`),
      }))}
      onChange={onChange}
    />
  );
}

const frame = (selected: boolean, deleted: boolean): CSSProperties => ({
  padding: "6px 8px",
  borderRadius: "var(--shape-sm)",
  outline: selected ? "2px solid var(--color-accent)" : undefined,
  background: deleted ? "var(--color-sunken)" : undefined,
});

export function SavedBlock({
  item,
  selected,
  editable,
  dirty,
  apply,
  onSelect,
  onInsert,
  onHand,
  handLabel,
  t,
}: {
  item: Extract<DocItem, { source: "saved" }>;
  selected: boolean;
  editable: boolean;
  dirty: boolean;
  apply(change: (draft: Draft<DocOperation>) => Draft<DocOperation>): void;
  onSelect(selection: Selection | undefined): void;
  onInsert(after: BlockView): void;
  onHand(block: BlockView): void;
  handLabel: string;
  t: Translate;
}) {
  const { block, saved, deleted } = item;
  const [editing, setEditing] = useState(false);
  const open = () => {
    onSelect({ source: "saved", id: block.id });
    setEditing(
      editable &&
        !deleted &&
        block.kind === "paragraph" &&
        block.runs.length > 0,
    );
  };
  return (
    <div role="listitem" style={frame(selected, deleted)}>
      <div
        role="button"
        tabIndex={0}
        aria-pressed={selected}
        onClick={open}
        style={
          deleted ? { opacity: 0.6, textDecoration: "line-through" } : undefined
        }
        onKeyDown={(event) => {
          if (event.target !== event.currentTarget || event.key !== "Enter")
            return;
          event.preventDefault();
          open();
        }}
      >
        {editing && selected && block.kind === "paragraph" ? (
          <ParagraphEditor
            text={textOf(block.runs)}
            style={block.style}
            alignment={block.alignment}
            label={t("docs.text")}
            onCommit={(text) => {
              setEditing(false);
              saved.kind === "paragraph" &&
                apply((draft) => editText(draft, saved, text));
            }}
            onCancel={() => setEditing(false)}
          />
        ) : (
          <BlockContent block={block} t={t} />
        )}
      </div>
      {item.changed && !deleted && <Badge>{t("docs.changed")}</Badge>}
      {deleted && <Badge>{t("docs.deleted")}</Badge>}
      {selected && (
        <SavedActions
          block={saved}
          style={block.kind === "paragraph" ? block.style : ""}
          deleted={deleted}
          editable={editable}
          dirty={dirty}
          apply={apply}
          onInsert={onInsert}
          onHand={onHand}
          handLabel={handLabel}
          t={t}
        />
      )}
    </div>
  );
}

function SavedActions({
  block,
  style,
  deleted,
  editable,
  dirty,
  apply,
  onInsert,
  onHand,
  handLabel,
  t,
}: {
  block: BlockView;
  // The paragraph style the draft shows.
  style: string;
  deleted: boolean;
  editable: boolean;
  dirty: boolean;
  apply(change: (draft: Draft<DocOperation>) => Draft<DocOperation>): void;
  onInsert(after: BlockView): void;
  onHand(block: BlockView): void;
  handLabel: string;
  t: Translate;
}) {
  return (
    <>
      <Toolbar label={t("docs.actions")}>
        {block.kind === "paragraph" && !deleted && (
          <StyleChoice
            value={style}
            disabled={!editable}
            onChange={(style) =>
              apply((draft) => setStyle(draft, block, style))
            }
            t={t}
          />
        )}
        <Button disabled={!editable || deleted} onClick={() => onInsert(block)}>
          {t("docs.insert")}
        </Button>
        <Button
          disabled={!editable}
          onClick={() =>
            apply((draft) =>
              deleted ? restore(draft, block) : remove(draft, block),
            )
          }
        >
          {t(deleted ? "docs.restore" : "docs.delete")}
        </Button>
        <Button disabled={dirty} onClick={() => onHand(block)}>
          {handLabel}
        </Button>
      </Toolbar>
      {block.kind === "paragraph" && !block.runs.length && (
        <Metadata>{t("docs.emptyHelp")}</Metadata>
      )}
    </>
  );
}

export function InsertedBlock({
  index,
  paragraph,
  selected,
  editable,
  apply,
  onSelect,
  t,
}: {
  index: number;
  paragraph: InsertedParagraph;
  selected: boolean;
  editable: boolean;
  apply(change: (draft: Draft<DocOperation>) => Draft<DocOperation>): void;
  onSelect(selection: Selection | undefined): void;
  t: Translate;
}) {
  const style = paragraph.style ?? "Normal";
  // A paragraph inserted from an action arrives selected, ready for typing.
  const [editing, setEditing] = useState(selected && editable);
  return (
    <div
      role="listitem"
      style={{
        ...frame(selected, false),
        background: "var(--color-info-wash)",
      }}
    >
      <div
        role="button"
        tabIndex={0}
        aria-pressed={selected}
        onClick={() => {
          onSelect({ source: "inserted", index });
          setEditing(editable);
        }}
      >
        {editing && selected ? (
          <ParagraphEditor
            text={paragraph.text}
            style={style}
            label={t("docs.text")}
            onCommit={(text) => {
              setEditing(false);
              apply((draft) => changeInserted(draft, index, { text }));
            }}
            onCancel={() => setEditing(false)}
          />
        ) : (
          <p style={paragraphStyle(style)}>
            {paragraph.text || (
              <span style={{ color: "var(--color-text-soft)" }}>
                {t("docs.empty")}
              </span>
            )}
          </p>
        )}
      </div>
      <Badge>{t("docs.new")}</Badge>
      {selected && (
        <Toolbar label={t("docs.actions")}>
          <StyleChoice
            value={style}
            disabled={!editable}
            onChange={(next) =>
              apply((draft) => changeInserted(draft, index, { style: next }))
            }
            t={t}
          />
          <Button
            disabled={!editable}
            onClick={() => {
              apply((draft) => changeInserted(draft, index, undefined));
              onSelect(undefined);
            }}
          >
            {t("docs.delete")}
          </Button>
        </Toolbar>
      )}
    </div>
  );
}

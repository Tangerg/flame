import { useMemo, useState, type CSSProperties, type ReactNode } from "react";
import type { Translate } from "@flame/component-sdk";
import {
  Button,
  Callout,
  Disclosure,
  Metadata,
  RecordButton,
  RecordGroup,
  RecordSummary,
  Select,
} from "@flame/ui";
import { PARAGRAPH_STYLES } from "../contracts/content";
import { LIMITS } from "../contracts/limits";
import type {
  BlockView,
  DocumentLayout,
  SheetCellView,
  SlideLayout,
  WorkbookLayout,
} from "../contracts/layouts";
import { revisionLabel } from "./archive";
import {
  compareBlocks,
  compareSheets,
  compareSlides,
  textChanges,
  type ElementChange,
  type Segment,
  type SlideChange,
} from "./compare";
import { blockText, styleOf } from "./docs-draft";
import type { LiveEditor } from "./live";
import { shownText } from "./sheets-grid";
import { chartText, frameText } from "./slides-presentation";

type ChangeLine = { label?: string; text: Segment[] };
// A change locates its target only when the shown revision still has it.
type ChangeItem<Target> = {
  key: string;
  title: string;
  status?: string;
  lines: ChangeLine[];
  target?: Target;
};

// Underline and strike-through carry the change without relying on color.
const MARKS = {
  added: {
    background: "var(--color-success-badge)",
    color: "var(--color-success-badge-text)",
    textDecorationLine: "underline",
  },
  removed: {
    background: "var(--color-negative-badge)",
    color: "var(--color-negative-badge-text)",
    textDecorationLine: "line-through",
  },
} satisfies Record<string, CSSProperties>;
const MUTED = { color: "var(--color-text-muted)" };
const ROW = {
  paddingInline: "var(--layout-row-inset, var(--layout-card-padding))",
  paddingBlock: "var(--layout-row-padding)",
};

function Redline({ text }: { text: Segment[] }) {
  return (
    <>
      {text.map((segment, index) =>
        segment.change === "added" ? (
          <ins key={index} style={MARKS.added}>
            {segment.text}
          </ins>
        ) : segment.change === "removed" ? (
          <del key={index} style={MARKS.removed}>
            {segment.text}
          </del>
        ) : (
          <span key={index}>{segment.text}</span>
        ),
      )}
    </>
  );
}

function ChangeRow<Target>({
  item,
  onOpen,
}: {
  item: ChangeItem<Target>;
  onOpen(target: Target): void;
}) {
  const summary = (
    <RecordSummary
      title={item.title}
      status={item.status && <span style={MUTED}>{item.status}</span>}
      description={
        item.lines.length > 0 &&
        item.lines.map((line, index) => (
          <span
            key={index}
            style={{ display: "block", whiteSpace: "pre-wrap" }}
          >
            {line.label && <span style={MUTED}>{line.label} </span>}
            <Redline text={line.text} />
          </span>
        ))
      }
    />
  );
  const target = item.target;
  return target === undefined ? (
    <div style={ROW}>{summary}</div>
  ) : (
    <RecordButton onClick={() => onOpen(target)}>{summary}</RecordButton>
  );
}

function ChangeList<Target>({
  items,
  onOpen,
  t,
}: {
  items: ChangeItem<Target>[];
  onOpen(target: Target): void;
  t: Translate;
}) {
  const [limit, setLimit] = useState<number>(LIMITS.pageSize);
  if (!items.length) return <Metadata>{t("changes.none")}</Metadata>;
  return (
    <>
      <RecordGroup variant="list" aria-label={t("changes.title")}>
        {items.slice(0, limit).map((item) => (
          <ChangeRow key={item.key} item={item} onOpen={onOpen} />
        ))}
      </RecordGroup>
      {items.length > limit && (
        <Button onClick={() => setLimit(limit + LIMITS.pageSize)}>
          {t("changes.more", {
            count: Math.min(LIMITS.pageSize, items.length - limit),
          })}
        </Button>
      )}
    </>
  );
}

// The editor's revision compared with an earlier one, by default its parent.
export function RevisionChanges<T, Operation>({
  live,
  dateTime,
  t,
  children,
}: {
  live: LiveEditor<T, Operation>;
  dateTime: Intl.DateTimeFormat;
  t: Translate;
  children(before: T, after: T): ReactNode;
}) {
  const { comparison } = live;
  const before = comparison.read?.value;
  const after = live.read?.value;
  return (
    <Disclosure
      title={t("changes.title")}
      open={comparison.open}
      onOpenChange={comparison.setOpen}
    >
      {comparison.base === undefined ? (
        <Metadata>{t("changes.first")}</Metadata>
      ) : (
        <>
          <Select
            label={t("changes.base")}
            value={comparison.base}
            onValueChange={comparison.choose}
            options={comparison.choices.map(({ id, revision }) => ({
              value: id,
              label: revision
                ? revisionLabel(revision, dateTime, t)
                : id.slice(0, 12),
            }))}
          />
          {comparison.read?.error && (
            <Callout kind="warning">
              {comparison.read.error}{" "}
              <Button onClick={live.retry}>{t("draft.retry")}</Button>
            </Callout>
          )}
          {before && after
            ? children(before, after)
            : !comparison.read?.error && (
                <Metadata role="status">{t("changes.loading")}</Metadata>
              )}
        </>
      )}
    </Disclosure>
  );
}

const textLine = (before: string, after: string): ChangeLine[] =>
  before === after ? [] : [{ text: textChanges(before, after) }];

export type SlideTarget = { slide: string; element?: string };

function elementItem(
  slide: string,
  title: string,
  change: ElementChange,
  t: Translate,
): ChangeItem<SlideTarget> {
  const { element } = change;
  const key = `${slide} ${element.id}`;
  const named = `${title} · ${element.name || t(`slides.kind.${element.kind}`)}`;
  if (change.type !== "changed")
    return {
      key,
      title: named,
      status: t(`changes.${change.type}`),
      lines: [],
      target:
        change.type === "added" ? { slide, element: element.id } : undefined,
    };
  const { before } = change;
  return {
    key,
    title: named,
    lines: [
      ...change.text.flatMap((run) => textLine(run.before, run.after)),
      ...(change.frame && before.frame && element.frame
        ? [
            {
              label: t("changes.frame"),
              text: textChanges(
                frameText(before.frame),
                frameText(element.frame),
              ),
            },
          ]
        : []),
      ...(change.chart && before.chart && element.chart
        ? [
            {
              label: t("changes.chart"),
              text: textChanges(
                chartText(before.chart.data),
                chartText(element.chart.data),
              ),
            },
          ]
        : []),
    ],
    target: { slide, element: element.id },
  };
}

function slideItems(
  change: SlideChange,
  t: Translate,
): ChangeItem<SlideTarget>[] {
  const { slide, status } = change;
  const title = [
    t("slides.slide", { number: slide.number }),
    ...(slide.name ? [slide.name] : []),
  ].join(" · ");
  const target = status.type === "removed" ? undefined : { slide: slide.id };
  return [
    ...(status.type === "kept"
      ? []
      : [
          {
            key: slide.id,
            title,
            status:
              status.type === "moved"
                ? t("changes.moved", { number: status.from })
                : t(`changes.${status.type}`),
            lines: [],
            target,
          },
        ]),
    ...(change.notes.length
      ? [
          {
            key: `${slide.id} notes`,
            title: `${title} · ${t("changes.notes")}`,
            lines: change.notes.flatMap((run) =>
              textLine(run.before, run.after),
            ),
            target,
          },
        ]
      : []),
    ...change.elements.map((element) =>
      elementItem(slide.id, title, element, t),
    ),
  ];
}

export function SlideChanges({
  before,
  after,
  onOpen,
  t,
}: {
  before: SlideLayout;
  after: SlideLayout;
  onOpen(target: SlideTarget): void;
  t: Translate;
}) {
  const items = useMemo(
    () =>
      compareSlides(before, after).flatMap((change) => slideItems(change, t)),
    [before, after, t],
  );
  return <ChangeList items={items} onOpen={onOpen} t={t} />;
}

// A formula shows with its calculated result when there is one.
function cellText(cell: SheetCellView | undefined): string {
  if (!cell) return "";
  const shown = shownText(cell.shown);
  if (cell.input?.type !== "formula") return shown;
  return shown ? `${cell.input.value} = ${shown}` : cell.input.value;
}

export type CellTarget = { sheet: string; ref: string };

export function SheetChanges({
  before,
  after,
  onOpen,
  t,
}: {
  before: WorkbookLayout;
  after: WorkbookLayout;
  onOpen(target: CellTarget): void;
  t: Translate;
}) {
  const items = useMemo(
    () =>
      compareSheets(before, after).flatMap(({ sheet, cells }) => {
        const shown = after.sheets.some((entry) => entry.id === sheet.id);
        return cells.map((cell): ChangeItem<CellTarget> => {
          const lines = textLine(cellText(cell.before), cellText(cell.after));
          return {
            key: `${sheet.id}!${cell.ref}`,
            title: t("sheets.op.setCell", { sheet: sheet.name, ref: cell.ref }),
            status: lines.length ? undefined : t("changes.type"),
            lines,
            target: shown ? { sheet: sheet.id, ref: cell.ref } : undefined,
          };
        });
      }),
    [before, after, t],
  );
  return <ChangeList items={items} onOpen={onOpen} t={t} />;
}

export function blockTitle(block: BlockView, t: Translate): string {
  if (block.kind === "table") return t("docs.table");
  if (block.kind === "other")
    return t("docs.other", { element: block.element });
  return styleName(styleOf(block.style), t);
}

const styleName = (style: string, t: Translate) =>
  (PARAGRAPH_STYLES as readonly string[]).includes(style)
    ? t(`docs.style.${style}`)
    : style;

function styleLine(
  before: BlockView,
  after: BlockView,
  t: Translate,
): ChangeLine[] {
  if (before.kind !== "paragraph" || after.kind !== "paragraph") return [];
  const was = styleOf(before.style);
  const now = styleOf(after.style);
  return was === now
    ? []
    : [
        {
          label: t("docs.style"),
          text: [
            { text: styleName(was, t), change: "removed" },
            { text: styleName(now, t), change: "added" },
          ],
        },
      ];
}

// Block identities are positions, so a removed block has no target and a
// list key cannot be its identity.
export function BlockChanges({
  before,
  after,
  onOpen,
  t,
}: {
  before: DocumentLayout;
  after: DocumentLayout;
  onOpen(block: string): void;
  t: Translate;
}) {
  const items = useMemo(
    () =>
      compareBlocks(before, after).map((change, index): ChangeItem<string> => {
        const { block } = change;
        const key = String(index);
        if (change.type !== "changed")
          return {
            key,
            title: blockTitle(block, t),
            status: t(`changes.${change.type}`),
            lines: blockText(block)
              ? [{ text: [{ text: blockText(block), change: change.type }] }]
              : [],
            target: change.type === "added" ? block.id : undefined,
          };
        // Unchanged text still shows, so a style change names its paragraph.
        const text = blockText(block);
        const was = blockText(change.before);
        const style = styleLine(change.before, block, t);
        return {
          key,
          title: blockTitle(block, t),
          status:
            was === text && !style.length ? t("changes.formatting") : undefined,
          lines: [
            ...(was !== text
              ? textLine(was, text)
              : text
                ? [{ text: [{ text }] }]
                : []),
            ...style,
          ],
          target: block.id,
        };
      }),
    [before, after, t],
  );
  return <ChangeList items={items} onOpen={onOpen} t={t} />;
}

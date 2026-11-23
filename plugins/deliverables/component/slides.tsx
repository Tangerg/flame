import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type PointerEvent,
} from "react";
import { failureText, type Translate } from "@flame/component-sdk";
import {
  Badge,
  Button,
  Callout,
  Facts,
  FormActions,
  FormField,
  Metadata,
  PanelSection,
  RecordGroup,
  Segmented,
  TextArea,
  TextField,
  Toolbar,
  copyText,
} from "@flame/ui";
import {
  slideLayoutSchema,
  type ChartData,
  type ElementView,
  type Frame,
  type SlideLayout,
  type SlideView,
} from "../contracts/content";
import { METHODS } from "../contracts/methods";
import type { DocumentRead } from "./types";
import type { DeliverablesState } from "./useDeliverables";
import {
  operationsJSON,
  preview,
  rebase,
  reconcile,
  record,
  revert,
  type Draft,
  type DraftOperation,
  type Reconciliation,
} from "./slides-draft";

const EMU_PER_INCH = 914400;
const NUDGE = EMU_PER_INCH / 10;

type LayoutRead = { value?: SlideLayout; error?: string };

// The bounded format process admits one operation at a time, so revisions are
// read in sequence rather than in parallel.
function useLayouts(state: DeliverablesState, documentId: string, wanted: string[]) {
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
          throw new Error("deliverables: slide layout differs from its revision");
        return { value: slideLayoutSchema.parse(JSON.parse(result.layoutJSON)) };
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

function inches(value: number): string {
  return String(Math.round((value / EMU_PER_INCH) * 1000) / 1000);
}

function excerpt(element: ElementView): string {
  const text = element.text
    .map((run) => run.text)
    .join("")
    .replace(/\s+/gu, " ")
    .trim();
  return text.length > 80 ? `${text.slice(0, 80)}…` : text;
}

function label(element: ElementView, t: Translate): string {
  return excerpt(element) || element.name || t(`slides.kind.${element.kind}`);
}

function selectionReference(
  document: DocumentRead,
  slide: SlideView,
  element: ElementView | undefined,
): string {
  return [
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
}

function editableTarget(event: KeyboardEvent): boolean {
  const target = event.target as HTMLElement;
  return (
    target.isContentEditable ||
    ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName)
  );
}

function Canvas({
  layout,
  slide,
  selected,
  editable,
  onSelect,
  onFrame,
  onDelete,
  t,
}: {
  layout: SlideLayout;
  slide: SlideView;
  selected?: string;
  editable: boolean;
  onSelect(id: string | undefined): void;
  onFrame(element: ElementView, frame: Frame): void;
  onDelete(element: ElementView): void;
  t: Translate;
}) {
  const surface = useRef<HTMLDivElement>(null);
  const [gesture, setGesture] = useState<{
    id: string;
    mode: "move" | "resize";
    pointer: number;
    x: number;
    y: number;
    start: Frame;
    frame: Frame;
  }>();
  const begin = (
    event: PointerEvent<HTMLElement>,
    element: ElementView,
    mode: "move" | "resize",
  ) => {
    event.stopPropagation();
    onSelect(element.id);
    if (!editable || !element.frame || event.button !== 0) return;
    event.currentTarget.setPointerCapture(event.pointerId);
    setGesture({
      id: element.id,
      mode,
      pointer: event.pointerId,
      x: event.clientX,
      y: event.clientY,
      start: element.frame,
      frame: element.frame,
    });
  };
  const move = (event: PointerEvent<HTMLElement>) => {
    if (!gesture || event.pointerId !== gesture.pointer || !surface.current)
      return;
    const scale = layout.width / surface.current.getBoundingClientRect().width;
    const dx = Math.round((event.clientX - gesture.x) * scale);
    const dy = Math.round((event.clientY - gesture.y) * scale);
    const { start } = gesture;
    setGesture({
      ...gesture,
      frame:
        gesture.mode === "move"
          ? { ...start, x: start.x + dx, y: start.y + dy }
          : {
              ...start,
              width: Math.max(0, start.width + dx),
              height: Math.max(0, start.height + dy),
            },
    });
  };
  const end = (event: PointerEvent<HTMLElement>, element: ElementView) => {
    if (!gesture || event.pointerId !== gesture.pointer) return;
    setGesture(undefined);
    if (
      gesture.frame.x !== gesture.start.x ||
      gesture.frame.y !== gesture.start.y ||
      gesture.frame.width !== gesture.start.width ||
      gesture.frame.height !== gesture.start.height
    )
      onFrame(element, gesture.frame);
  };
  const keys = (event: KeyboardEvent<HTMLElement>, element: ElementView) => {
    if (event.key === "Escape") {
      onSelect(undefined);
      return;
    }
    if (!editable) return;
    if (event.key === "Delete" || event.key === "Backspace") {
      event.preventDefault();
      onDelete(element);
      return;
    }
    const step = event.shiftKey ? EMU_PER_INCH : NUDGE;
    const delta = {
      ArrowLeft: [-step, 0],
      ArrowRight: [step, 0],
      ArrowUp: [0, -step],
      ArrowDown: [0, step],
    }[event.key];
    if (!delta || !element.frame) return;
    event.preventDefault();
    onFrame(element, {
      ...element.frame,
      x: element.frame.x + delta[0]!,
      y: element.frame.y + delta[1]!,
    });
  };
  const percent = (value: number, total: number) => `${(value / total) * 100}%`;
  return (
    <div
      ref={surface}
      role="group"
      aria-label={t("slides.canvas", { number: slide.number })}
      aria-description={t("slides.keyboard")}
      onPointerDown={() => onSelect(undefined)}
      style={{
        position: "relative",
        width: "100%",
        aspectRatio: `${layout.width} / ${layout.height}`,
        overflow: "hidden",
        background: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        borderRadius: "var(--radius-md, 6px)",
        touchAction: "none",
      }}
    >
      {slide.elements.map((element) => {
        const frame =
          gesture?.id === element.id ? gesture.frame : element.frame;
        if (!frame) return null;
        const active = element.id === selected;
        return (
          <button
            key={element.id}
            type="button"
            aria-pressed={active}
            aria-label={`${t(`slides.kind.${element.kind}`)} · ${label(element, t)}`}
            onPointerDown={(event) => begin(event, element, "move")}
            onPointerMove={move}
            onPointerUp={(event) => end(event, element)}
            onPointerCancel={() => setGesture(undefined)}
            onKeyDown={(event) => keys(event, element)}
            onFocus={() => onSelect(element.id)}
            style={{
              position: "absolute",
              left: percent(frame.x, layout.width),
              top: percent(frame.y, layout.height),
              width: percent(frame.width, layout.width),
              height: percent(frame.height, layout.height),
              minWidth: 6,
              minHeight: 6,
              transform: element.rotation
                ? `rotate(${element.rotation}deg)`
                : undefined,
              padding: 2,
              margin: 0,
              overflow: "hidden",
              textAlign: "start",
              font: "inherit",
              fontSize: "0.7rem",
              lineHeight: 1.25,
              color: "var(--color-text-soft)",
              cursor: editable && element.frame ? "move" : "pointer",
              background:
                element.kind === "chart" || element.kind === "table"
                  ? "var(--color-info-wash)"
                  : element.kind === "picture"
                    ? "var(--color-sunken)"
                    : element.kind === "text"
                      ? "transparent"
                      : "var(--color-surface-2)",
              border: `1px ${active ? "solid" : "dashed"} ${
                active ? "var(--color-accent)" : "var(--color-border-strong)"
              }`,
              outline: active ? "2px solid var(--color-accent)" : "none",
              outlineOffset: 1,
              zIndex: active ? 1 : undefined,
            }}
          >
            {label(element, t)}
            {active && editable && element.frame && (
              <span
                aria-hidden
                onPointerDown={(event) => begin(event, element, "resize")}
                onPointerMove={(event) => {
                  event.stopPropagation();
                  move(event);
                }}
                onPointerUp={(event) => {
                  event.stopPropagation();
                  end(event, element);
                }}
                style={{
                  position: "absolute",
                  right: 0,
                  bottom: 0,
                  width: 10,
                  height: 10,
                  cursor: "nwse-resize",
                  background: "var(--color-accent)",
                }}
              />
            )}
          </button>
        );
      })}
    </div>
  );
}

function FrameFields({
  frame,
  editable,
  onCommit,
  t,
}: {
  frame: Frame;
  editable: boolean;
  onCommit(frame: Frame): void;
  t: Translate;
}) {
  const fields = ["x", "y", "width", "height"] as const;
  const [values, setValues] = useState(() =>
    Object.fromEntries(fields.map((field) => [field, inches(frame[field])])),
  );
  const [invalid, setInvalid] = useState(false);
  useEffect(() => {
    setValues(
      Object.fromEntries(fields.map((field) => [field, inches(frame[field])])),
    );
    setInvalid(false);
  }, [frame.x, frame.y, frame.width, frame.height]);
  const commit = () => {
    const next = Object.fromEntries(
      fields.map((field) => [
        field,
        Math.round(Number(values[field]) * EMU_PER_INCH),
      ]),
    ) as Frame;
    if (
      fields.some(
        (field) =>
          values[field]!.trim() === "" || !Number.isSafeInteger(next[field]),
      ) ||
      next.width < 0 ||
      next.height < 0
    ) {
      setInvalid(true);
      return;
    }
    setInvalid(false);
    if (fields.some((field) => next[field] !== frame[field])) onCommit(next);
  };
  return (
    <RecordGroup>
      {fields.map((field) => (
        <FormField key={field} label={t(`slides.frame.${field}`)}>
          <TextField
            type="number"
            step="0.01"
            inputMode="decimal"
            value={values[field]}
            readOnly={!editable}
            invalid={invalid}
            onChange={(event) =>
              setValues({ ...values, [field]: event.target.value })
            }
            onBlur={commit}
            onKeyDown={(event) => {
              if (event.key === "Enter") commit();
            }}
          />
        </FormField>
      ))}
      {invalid && <Callout kind="warning">{t("slides.frame.invalid")}</Callout>}
    </RecordGroup>
  );
}

function RunField({
  label: title,
  text,
  editable,
  onCommit,
}: {
  label: string;
  text: string;
  editable: boolean;
  onCommit(text: string): void;
}) {
  const [value, setValue] = useState(text);
  useEffect(() => setValue(text), [text]);
  return (
    <FormField label={title}>
      <TextArea
        autosize
        size="sm"
        value={value}
        readOnly={!editable}
        onChange={(event) => setValue(event.target.value)}
        onBlur={() => {
          if (value !== text) onCommit(value);
        }}
      />
    </FormField>
  );
}

function ChartFields({
  chart,
  editable,
  onCommit,
  t,
}: {
  chart: NonNullable<ElementView["chart"]>;
  editable: boolean;
  onCommit(data: ChartData): void;
  t: Translate;
}) {
  const source = (data: ChartData) => ({
    categories: [...data.categories],
    names: data.series.map((series) => series.name),
    values: data.series.map((series) =>
      series.values.map((value) => (value === null ? "" : String(value))),
    ),
  });
  const [grid, setGrid] = useState(() => source(chart.data));
  const [invalid, setInvalid] = useState(false);
  useEffect(() => {
    setGrid(source(chart.data));
    setInvalid(false);
  }, [chart.data]);
  const writable = editable && chart.workbook !== "mixed";
  const apply = () => {
    const values = grid.values.map((series) =>
      series.map((value) => (value.trim() === "" ? null : Number(value))),
    );
    if (values.some((series) => series.some((value) => value !== null && !Number.isFinite(value)))) {
      setInvalid(true);
      return;
    }
    setInvalid(false);
    onCommit({
      categories: grid.categories,
      series: grid.names.map((name, index) => ({
        name,
        values: values[index]!,
      })),
    });
  };
  const cell = { padding: "2px 4px", minWidth: "5rem" };
  return (
    <RecordGroup>
      {chart.workbook === "mixed" && (
        <Callout kind="note">{t("slides.chart.mixed")}</Callout>
      )}
      <div style={{ overflowX: "auto" }}>
        <table style={{ borderCollapse: "collapse" }}>
          <thead>
            <tr>
              <th scope="col" style={cell}>
                {t("slides.chart.category")}
              </th>
              {grid.names.map((name, series) => (
                <th key={series} scope="col" style={cell}>
                  <TextField
                    size="sm"
                    aria-label={t("slides.chart.series", { number: series + 1 })}
                    value={name}
                    readOnly={!writable}
                    onChange={(event) => {
                      const names = [...grid.names];
                      names[series] = event.target.value;
                      setGrid({ ...grid, names });
                    }}
                  />
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {grid.categories.map((category, point) => (
              <tr key={point}>
                <th scope="row" style={cell}>
                  <TextField
                    size="sm"
                    aria-label={t("slides.chart.categoryNumber", {
                      number: point + 1,
                    })}
                    value={category}
                    readOnly={!writable}
                    onChange={(event) => {
                      const categories = [...grid.categories];
                      categories[point] = event.target.value;
                      setGrid({ ...grid, categories });
                    }}
                  />
                </th>
                {grid.values.map((series, index) => (
                  <td key={index} style={cell}>
                    <TextField
                      size="sm"
                      inputMode="decimal"
                      aria-label={`${grid.names[index]} · ${category}`}
                      value={series[point]}
                      readOnly={!writable}
                      invalid={invalid}
                      onChange={(event) => {
                        const values = grid.values.map((entry) => [...entry]);
                        values[index]![point] = event.target.value;
                        setGrid({ ...grid, values });
                      }}
                    />
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {invalid && <Callout kind="warning">{t("slides.chart.invalid")}</Callout>}
      <Button disabled={!writable} onClick={apply}>
        {t("slides.chart.apply")}
      </Button>
    </RecordGroup>
  );
}

function describe(operation: DraftOperation, base: SlideLayout | undefined, t: Translate) {
  if (operation.type === "replaceText")
    return t("slides.op.replaceText", { text: operation.text.slice(0, 60) });
  const element = base?.slides
    .flatMap((slide) => slide.elements)
    .find((entry) => entry.id === operation.id);
  const name = element ? label(element, t) : operation.id;
  return t(`slides.op.${operation.type}`, { name });
}

function Reconcile({
  base,
  head,
  draft,
  onRebase,
  onDiscard,
  t,
}: {
  base: SlideLayout;
  head: { id: string; layout: SlideLayout };
  draft: Draft;
  onRebase(draft: Draft): void;
  onDiscard(): void;
  t: Translate;
}) {
  const results = useMemo(
    () => reconcile(base, head.layout, draft.operations),
    [base, head.layout, draft.operations],
  );
  const [mine, setMine] = useState<ReadonlySet<number>>(new Set());
  const show = (value: unknown) =>
    typeof value === "string" ? value : JSON.stringify(value);
  return (
    <RecordGroup>
      <Metadata>{t("slides.reconcile.help")}</Metadata>
      {results.map((result: Reconciliation, index) => (
        <RecordGroup key={index}>
          <Metadata>
            {describe(result.operation, base, t)} ·{" "}
            <Badge>{t(`slides.status.${result.status}`)}</Badge>
          </Metadata>
          {result.status === "conflict" && (
            <>
              <Facts
                values={[
                  [
                    t("slides.mine"),
                    result.operation.type === "deleteElement"
                      ? t("slides.kind.deleted")
                      : show(
                          "text" in result.operation
                            ? result.operation.text
                            : "frame" in result.operation
                              ? result.operation.frame
                              : result.operation.data,
                        ),
                  ],
                  [t("slides.theirs"), show(result.current)],
                ]}
              />
              <Segmented
                ariaLabel={t("slides.reconcile.choice")}
                value={mine.has(index) ? "mine" : "theirs"}
                options={[
                  { value: "theirs", label: t("slides.keepCurrent") },
                  { value: "mine", label: t("slides.keepMine") },
                ]}
                onChange={(value) => {
                  const next = new Set(mine);
                  if (value === "mine") next.add(index);
                  else next.delete(index);
                  setMine(next);
                }}
              />
            </>
          )}
        </RecordGroup>
      ))}
      <FormActions label={t("slides.reconcile.actions")}>
        <Button onClick={onDiscard}>{t("slides.discard")}</Button>
        <Button
          variant="primary"
          onClick={() => onRebase(rebase(head.id, results, mine))}
        >
          {t("slides.rebase")}
        </Button>
      </FormActions>
    </RecordGroup>
  );
}

export function SlideStudio({
  state,
  document,
  t,
}: {
  state: DeliverablesState;
  document: DocumentRead;
  t: Translate;
}) {
  const documentId = document.document.id;
  const head = document.document.head;
  const viewed = document.revision.id;
  const [draft, setDraft] = useState<Draft>();
  const [saving, setSaving] = useState(false);
  const [slideId, setSlideId] = useState<string>();
  const [elementId, setElementId] = useState<string>();
  const [removal, setRemoval] = useState<string>();
  const [copied, setCopied] = useState<"copied" | "failed">();
  const pending =
    draft && draft.base !== viewed && draft.operations.length
      ? draft.base
      : undefined;
  useEffect(() => {
    if (draft && draft.base !== viewed && !draft.operations.length)
      setDraft(undefined);
  }, [draft, viewed]);
  const layouts = useLayouts(
    state,
    documentId,
    pending ? [viewed, pending] : [viewed],
  );
  const read = layouts.get(viewed);
  const baseRead = pending ? layouts.get(pending) : undefined;
  const editing = draft?.base === viewed;
  const editable =
    viewed === head && !state.blocked && !saving && (!draft || editing);
  const layout = useMemo(
    () =>
      read?.value && editing ? preview(read.value, draft.operations) : read?.value,
    [read?.value, editing, draft?.operations],
  );
  const slide =
    layout?.slides.find((entry) => entry.id === slideId) ?? layout?.slides[0];
  const element = slide?.elements.find((entry) => entry.id === elementId);
  const original = read?.value?.slides
    .flatMap((entry) => entry.elements)
    .find((entry) => entry.id === elementId);
  const dirty = !!draft?.operations.length;
  const changed = (id: string) =>
    !!draft?.operations.some(
      (operation) =>
        operation.id === id ||
        original?.text.some((run) => run.id === operation.id),
    );

  const change = (operation: DraftOperation, target?: ElementView) => {
    if (!editable) return;
    setDraft((current) =>
      record(current ?? { base: viewed, operations: [], undo: [] }, operation, target),
    );
  };
  const setFrame = (target: ElementView, frame: Frame) => {
    const source = read?.value?.slides
      .flatMap((entry) => entry.elements)
      .find((entry) => entry.id === target.id)?.frame;
    if (source) change({ type: "setFrame", id: target.id, expect: source, frame });
  };
  const setText = (id: string, expect: string, text: string) =>
    change({ type: "replaceText", id, expect, text });
  const remove = (target: ElementView) => {
    change({ type: "deleteElement", id: target.id }, target);
    setElementId(undefined);
  };
  const undo = () =>
    setDraft((current) =>
      current?.undo.length
        ? {
            base: current.base,
            operations: current.undo.at(-1)!,
            undo: current.undo.slice(0, -1),
          }
        : current,
    );

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
          state.selectRevision(value, { id: documentId, revision: captured.base });
        },
      );
      if (!revision) void state.refresh();
    } finally {
      setSaving(false);
    }
  };
  const restructure = (operations: unknown[]) => {
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

  return (
    <PanelSection title={t("slides.title")} description={t("slides.help")}>
      {viewed !== head && !draft && (
        <Callout kind="note">{t("slides.historical")}</Callout>
      )}
      {editing && head !== viewed && (
        <Callout kind="warning">
          {t("slides.newer")}{" "}
          <Button
            onClick={() => state.open({ id: documentId, revision: head })}
          >
            {t("slides.compare")}
          </Button>
        </Callout>
      )}
      {pending && (
        <RecordGroup>
          {viewed === head ? (
            baseRead?.value && read?.value ? (
              <Reconcile
                base={baseRead.value}
                head={{ id: head, layout: read.value }}
                draft={draft!}
                onRebase={setDraft}
                onDiscard={() => setDraft(undefined)}
                t={t}
              />
            ) : (
              <Metadata role="status">{t("slides.loading")}</Metadata>
            )
          ) : (
            <Callout kind="note">
              {t("slides.draftElsewhere")}{" "}
              <Button
                onClick={() => state.open({ id: documentId, revision: pending })}
              >
                {t("slides.openDraft")}
              </Button>
            </Callout>
          )}
        </RecordGroup>
      )}
      {(read?.error || baseRead?.error) && (
        <Callout kind="warning">
          {read?.error ?? baseRead?.error}{" "}
          <Button
            onClick={() => {
              if (read?.error) layouts.retry(viewed);
              if (pending && baseRead?.error) layouts.retry(pending);
            }}
          >
            {t("slides.retry")}
          </Button>
        </Callout>
      )}
      {!read && <Metadata role="status">{t("slides.loading")}</Metadata>}
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
                    setRemoval(undefined);
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
                setCopied(undefined);
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
                    setCopied(undefined);
                  }}
                >
                  {t(`slides.kind.${entry.kind}`)} · {label(entry, t)}
                  {!entry.frame && ` · ${t("slides.noFrame")}`}
                  {changed(entry.id) && <Badge>{t("slides.changed")}</Badge>}
                </Button>
              ))}
            </RecordGroup>
          </div>
          <div style={{ flex: "2 1 16rem", minWidth: 0 }}>
            {element ? (
              <RecordGroup>
                <Facts
                  values={[
                    [t("slides.element"), t(`slides.kind.${element.kind}`)],
                    [t("slides.name"), element.name || null],
                  ]}
                />
                {element.frameSource && element.frameSource !== "slide" && (
                  <Metadata>
                    {t(`slides.inherited.${element.frameSource}`)}
                  </Metadata>
                )}
                {element.frame && (
                  <FrameFields
                    frame={element.frame}
                    editable={editable}
                    onCommit={(frame) => setFrame(element, frame)}
                    t={t}
                  />
                )}
                {element.text.map((run, index) => {
                  const saved =
                    original?.text.find((entry) => entry.id === run.id)?.text ??
                    run.text;
                  return (
                    <RunField
                      key={run.id}
                      label={t("slides.run", { number: index + 1 })}
                      text={run.text}
                      editable={editable}
                      onCommit={(text) => setText(run.id, saved, text)}
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
                    onCommit={(data) =>
                      change({
                        type: "setChartData",
                        id: element.id,
                        expect: original.chart!.data,
                        data,
                      })
                    }
                    t={t}
                  />
                )}
                <Toolbar label={t("slides.elementActions")}>
                  <Button disabled={!editable} onClick={() => remove(element)}>
                    {t("slides.delete")}
                  </Button>
                  <Button
                    disabled={!editing || !changed(element.id) || !original}
                    onClick={() => {
                      if (original)
                        setDraft((current) => current && revert(current, original));
                    }}
                  >
                    {t("slides.revert")}
                  </Button>
                  <Button
                    onClick={() => {
                      const unsavedSlide = read?.value?.slides.find(
                        (entry) => entry.id === slide.id,
                      );
                      if (!unsavedSlide || !original) return;
                      void copyText(
                        selectionReference(document, unsavedSlide, original),
                      ).then((done) => setCopied(done ? "copied" : "failed"));
                    }}
                  >
                    {t("slides.reference")}
                  </Button>
                </Toolbar>
                {changed(element.id) && (
                  <Metadata>{t("slides.reference.unsaved")}</Metadata>
                )}
                {copied && (
                  <Metadata role="status">
                    {t(`slides.reference.${copied}`)}
                  </Metadata>
                )}
              </RecordGroup>
            ) : (
              <RecordGroup>
                <Metadata>{t("slides.select")}</Metadata>
                {slide.notes.map((run, index) => {
                  const saved =
                    read?.value?.slides
                      .find((entry) => entry.id === slide.id)
                      ?.notes.find((entry) => entry.id === run.id)?.text ??
                    run.text;
                  return (
                    <RunField
                      key={run.id}
                      label={t("slides.notes", { number: index + 1 })}
                      text={run.text}
                      editable={editable}
                      onCommit={(text) => setText(run.id, saved, text)}
                    />
                  );
                })}
                <Toolbar label={t("slides.slideActions")}>
                  <Button
                    disabled={!editable || dirty || slide.number === 1}
                    onClick={() => move(-1)}
                  >
                    {t("slides.moveUp")}
                  </Button>
                  <Button
                    disabled={
                      !editable || dirty || slide.number === layout.slides.length
                    }
                    onClick={() => move(1)}
                  >
                    {t("slides.moveDown")}
                  </Button>
                  <Button
                    disabled={!editable || dirty}
                    onClick={() =>
                      restructure([{ type: "duplicateSlide", id: slide.id }])
                    }
                  >
                    {t("slides.duplicate")}
                  </Button>
                  <Button
                    disabled={!editable || dirty || layout.slides.length < 2}
                    onClick={() => setRemoval(slide.id)}
                  >
                    {t("slides.deleteSlide")}
                  </Button>
                  <Button
                    onClick={() => {
                      const saved = read?.value?.slides.find(
                        (entry) => entry.id === slide.id,
                      );
                      if (!saved) return;
                      void copyText(
                        selectionReference(document, saved, undefined),
                      ).then((done) => setCopied(done ? "copied" : "failed"));
                    }}
                  >
                    {t("slides.reference")}
                  </Button>
                </Toolbar>
                {dirty && <Metadata>{t("slides.structureBlocked")}</Metadata>}
                {removal === slide.id && (
                  <RecordGroup>
                    <Callout kind="warning">
                      {t("slides.deleteSlide.confirm", { number: slide.number })}
                    </Callout>
                    <FormActions label={t("slides.slideActions")}>
                      <Button onClick={() => setRemoval(undefined)}>
                        {t("cancel")}
                      </Button>
                      <Button
                        disabled={!editable || dirty}
                        onClick={() => {
                          setRemoval(undefined);
                          restructure([{ type: "deleteSlide", id: slide.id }]);
                        }}
                      >
                        {t("slides.deleteSlide")}
                      </Button>
                    </FormActions>
                  </RecordGroup>
                )}
                {copied && (
                  <Metadata role="status">
                    {t(`slides.reference.${copied}`)}
                  </Metadata>
                )}
              </RecordGroup>
            )}
          </div>
        </div>
      )}
      {editing && (
        <Toolbar label={t("slides.draft")}>
          <Metadata role="status">
            {saving
              ? t("slides.saving")
              : dirty
                ? t("slides.unsaved", { count: draft.operations.length })
                : t("slides.clean")}
          </Metadata>
          <Button disabled={!draft.undo.length || saving} onClick={undo}>
            {t("slides.undo")}
          </Button>
          <Button disabled={saving} onClick={() => setDraft(undefined)}>
            {t("slides.discard")}
          </Button>
          <Button
            variant="primary"
            disabled={!dirty || !editable}
            onClick={() => {
              void save();
            }}
          >
            {t("slides.save")}
          </Button>
        </Toolbar>
      )}
    </PanelSection>
  );
}

import { useMemo, useRef, useState } from "react";
import type { ComposerReference, Translate } from "@flame/component-sdk";
import {
  Button,
  Metadata,
  PanelSection,
  Segmented,
  TextField,
  Toolbar,
} from "@flame/ui";
import { type CellValue } from "../contracts/content";
import {
  workbookLayoutSchema,
  type SheetView,
  type WorkbookLayout,
} from "../contracts/layouts";
import { METHODS } from "../contracts/methods";
import type { DocumentRead } from "./types";
import type { Draft, DraftUpdate } from "./live-draft";
import {
  DraftToolbar,
  Reconcile,
  RevisionNotices,
  useHandoff,
  useLiveDraft,
  useProjection,
  type LiveState,
} from "./live";
import {
  cellId,
  entryText,
  parseEntry,
  reconcile,
  record,
  withCurrent,
  type CellOperation,
} from "./sheets-draft";
import {
  Grid,
  bounds,
  lockOf,
  pointOf,
  refOf,
  shownText,
  within,
  type Entry,
  type Point,
} from "./sheets-grid";

const REFERENCE_CELLS = 200;

function selectionReference(
  document: DocumentRead,
  sheet: SheetView,
  range: { start: Point; end: Point },
): ComposerReference {
  const area =
    range.start.row === range.end.row && range.start.column === range.end.column
      ? refOf(range.start)
      : `${refOf(range.start)}:${refOf(range.end)}`;
  const cells = sheet.cells.filter((cell) => within(range, pointOf(cell.ref)));
  const listed = cells.slice(0, REFERENCE_CELLS).map((cell) => {
    const shown = shownText(cell.shown);
    const formula =
      cell.input?.type === "formula" ? cell.input.value : undefined;
    return `${cell.ref}\t${formula && shown ? `${formula} → ${shown}` : (formula ?? shown)}`;
  });
  return {
    label: [document.document.title, sheet.name, area]
      .join(" · ")
      .replace(/\s+/gu, " ")
      .trim(),
    text: [
      "Flame deliverable selection",
      `Document: ${JSON.stringify(document.document.title)} (${document.document.id})`,
      `Revision: ${document.revision.id}`,
      `Sheet: ${JSON.stringify(sheet.name)} (${sheet.id})`,
      `Range: ${area}`,
      ...(listed.length
        ? ["Cells with values:", ...listed]
        : ["The range is empty."]),
      ...(cells.length > REFERENCE_CELLS
        ? [`${cells.length - REFERENCE_CELLS} more cells are not listed.`]
        : []),
      "Read this exact revision with the Deliverables sheets tool and carry each cell's expect when editing with setCell.",
    ].join("\n"),
  };
}

export function recalculate(
  state: Pick<LiveState, "run" | "selectRevision">,
  document: DocumentRead,
) {
  const captured = { id: document.document.id, revision: document.revision.id };
  void state.run(
    METHODS.recalculate,
    { id: captured.id, expectedRevision: captured.revision },
    (value) => state.selectRevision(value, captured),
  );
}

function SheetReconcile({
  head,
  layout,
  draft,
  onRebase,
  onDiscard,
  t,
}: {
  head: string;
  layout: WorkbookLayout;
  draft: Draft<CellOperation>;
  onRebase(draft: Draft<CellOperation>): void;
  onDiscard(): void;
  t: Translate;
}) {
  const results = useMemo(
    () => reconcile(layout, draft.operations),
    [layout, draft.operations],
  );
  const describe = (operation: CellOperation) => {
    const separator = operation.id.lastIndexOf("!");
    const sheet = layout.sheets.find(
      (entry) => entry.id === operation.id.slice(0, separator),
    );
    return t("sheets.op.setCell", {
      sheet: sheet?.name ?? "",
      ref: operation.id.slice(separator + 1),
    });
  };
  return (
    <Reconcile
      results={results}
      head={head}
      describe={describe}
      mine={(operation) => entryText(operation.value)}
      theirs={(result) =>
        entryText((result.current as { input?: CellValue }).input)
      }
      withCurrent={withCurrent}
      onRebase={onRebase}
      onDiscard={onDiscard}
      t={t}
    />
  );
}

// A reference always names the saved revision, never unsaved draft values.
function SheetActions({
  state,
  document,
  sheet,
  range,
  dirty,
  current,
  handoff,
  t,
}: {
  state: LiveState;
  document: DocumentRead;
  sheet: SheetView;
  range: { start: Point; end: Point };
  dirty: boolean;
  current: boolean;
  handoff: ReturnType<typeof useHandoff>;
  t: Translate;
}) {
  const uncalculated = sheet.cells.some(
    (cell) => cell.input?.type === "formula" && cell.shown.type === "blank",
  );
  return (
    <>
      <Toolbar label={t("sheets.actions")}>
        <Button
          disabled={dirty}
          onClick={() =>
            handoff.hand(selectionReference(document, sheet, range))
          }
        >
          {handoff.label}
        </Button>
        {uncalculated && (
          <Button
            disabled={dirty || state.blocked || !current}
            onClick={() => recalculate(state, document)}
          >
            {t("preview.recalculate")}
          </Button>
        )}
      </Toolbar>
      {uncalculated && <Metadata>{t("sheets.pending")}</Metadata>}
      {dirty && <Metadata>{t("handoff.unsaved")}</Metadata>}
      {handoff.status}
    </>
  );
}

export function WorkbookStudio({
  state,
  document,
  draft,
  onDraft,
  t,
}: {
  state: LiveState;
  document: DocumentRead;
  draft: Draft<CellOperation> | undefined;
  onDraft(update: DraftUpdate<CellOperation>): void;
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
    apply,
    undo,
    save,
  } = useLiveDraft(state, document, draft, onDraft);
  const handoff = useHandoff(state, t);
  const projections = useProjection(
    state,
    documentId,
    pending ? [viewed, pending] : [viewed],
    METHODS.sheets,
    workbookLayoutSchema,
  );
  const read = projections.get(viewed);
  const baseRead = pending ? projections.get(pending) : undefined;
  const layout = read?.value;
  const [sheetId, setSheetId] = useState<string>();
  const [anchor, setAnchor] = useState<Point>({ row: 1, column: 1 });
  const [focus, setFocus] = useState<Point>({ row: 1, column: 1 });
  const [editing, setEditing] = useState<Entry>();
  const entry = useRef(editing);
  entry.current = editing;
  const sheet =
    layout?.sheets.find((entry) => entry.id === sheetId) ?? layout?.sheets[0];
  const edits = useMemo(() => {
    const prefix = sheet ? `${sheet.id}!` : undefined;
    return new Map(
      (editedDraft?.operations ?? [])
        .filter((operation) => prefix && operation.id.startsWith(prefix))
        .map((operation) => [
          operation.id.slice(prefix!.length),
          operation.value,
        ]),
    );
  }, [editedDraft, sheet]);
  const savedCell = (ref: string) =>
    sheet?.cells.find((cell) => cell.ref === ref);
  const set = (refs: string[], value: CellValue) => {
    if (!sheet) return;
    const changes = refs
      .filter((ref) => !lockOf(sheet, pointOf(ref)))
      .map((ref) => {
        const original = savedCell(ref);
        return {
          operation: {
            type: "setCell" as const,
            id: cellId(sheet.id, ref),
            expect: original?.expect ?? "",
            value,
          },
          original,
        };
      });
    if (changes.length) apply((current) => record(current, changes));
  };
  const select = (next: Point, extend: boolean) => {
    setFocus(next);
    if (!extend) setAnchor(next);
    handoff.clear();
  };
  // Enter, Tab and blur can all end one entry; the first ends it.
  const commit = (next?: Point) => {
    const active = entry.current;
    if (!active) return;
    entry.current = undefined;
    setEditing(undefined);
    set([active.ref], parseEntry(active.text));
    if (next) select(next, false);
  };
  const edit = (next: Entry | undefined) => {
    entry.current = next;
    setEditing(next);
  };
  const range = bounds(anchor, focus);
  const focusRef = refOf(focus);
  const barText =
    editing?.ref === focusRef
      ? editing.text
      : entryText(edits.get(focusRef) ?? savedCell(focusRef)?.input);
  const focusLock = sheet && lockOf(sheet, focus);

  return (
    <PanelSection title={t("sheets.title")} description={t("sheets.help")}>
      <RevisionNotices
        t={t}
        loading={t("sheets.loading")}
        viewed={viewed}
        head={head}
        draft={draft}
        editing={editedDraft !== undefined}
        pending={pending}
        read={read}
        baseRead={baseRead}
        reconcile={(_base, current) =>
          draft && (
            <SheetReconcile
              head={head}
              layout={current}
              draft={draft}
              onRebase={setDraft}
              onDiscard={() => setDraft(undefined)}
              t={t}
            />
          )
        }
        onOpen={(revision) => state.open({ id: documentId, revision })}
        onRetry={() => {
          if (read?.error) projections.retry(viewed);
          if (pending && baseRead?.error) projections.retry(pending);
        }}
      />
      {layout && sheet && (
        <>
          {layout.sheets.length > 1 && (
            <Segmented
              ariaLabel={t("sheets.list")}
              value={sheet.id}
              options={layout.sheets.map((entry) => ({
                value: entry.id,
                label: entry.name,
              }))}
              onChange={(value) => {
                setSheetId(value);
                edit(undefined);
                select({ row: 1, column: 1 }, false);
              }}
            />
          )}
          <TextField
            aria-label={t("sheets.formula", { ref: focusRef })}
            value={barText}
            readOnly={!editable || !!focusLock}
            onChange={(event) =>
              edit({ ref: focusRef, text: event.target.value, source: "bar" })
            }
            onBlur={() => commit()}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                commit();
              } else if (event.key === "Escape") edit(undefined);
            }}
          />
          {focusLock && <Metadata>{t(`sheets.locked.${focusLock}`)}</Metadata>}
          <Grid
            sheet={sheet}
            edits={edits}
            anchor={anchor}
            focus={focus}
            editing={editing}
            editable={editable}
            onSelect={select}
            onEdit={edit}
            onCommit={commit}
            onClear={() =>
              // Blank cells stay blank, so only cells holding content change.
              set(
                [
                  ...new Set([
                    ...sheet.cells.map((cell) => cell.ref),
                    ...edits.keys(),
                  ]),
                ].filter((ref) => within(range, pointOf(ref))),
                { type: "blank" },
              )
            }
            onUndo={undo}
            t={t}
          />
          <Metadata>{t("sheets.keyboard")}</Metadata>
          <SheetActions
            state={state}
            document={document}
            sheet={sheet}
            range={range}
            dirty={dirty}
            current={viewed === head}
            handoff={handoff}
            t={t}
          />
        </>
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

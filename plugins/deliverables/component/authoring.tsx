import { useMemo, useState } from "react";
import { failureText, type Translate } from "@flame/component-sdk";
import { parseJSON, projectNumber } from "@qimu/sdk/json";
import {
  Button,
  Callout,
  Disclosure,
  Form,
  FormActions,
  FormField,
  FormGroup,
  FormRow,
  Metadata,
  RecordBody,
  RecordGroup,
  Select,
  TextArea,
  TextField,
  Toolbar,
} from "@flame/ui";
import {
  documentContentSchema,
  workbookContentSchema,
  presentationContentSchema,
  documentEditSchema,
  workbookEditSchema,
  presentationEditSchema,
  pdfEditSchema,
  cellValueSchema,
  type FormatCatalog,
  type Inspection,
  type CellValue,
  type PresentationContent,
} from "../contracts/content";
import { LIMITS } from "../contracts/limits";
import { METHODS } from "../contracts/methods";
import { projectNumbers } from "../contracts/json";
import type { DeliverablesState } from "./useDeliverables";
import {
  emptyProvenance,
  type DocumentRead,
  type OfficeFormat,
  type Provenance,
} from "./types";

const creationSchemas = {
  docx: documentContentSchema,
  xlsx: workbookContentSchema,
  pptx: presentationContentSchema,
};
const editSchemas = {
  docx: documentEditSchema,
  xlsx: workbookEditSchema,
  pptx: presentationEditSchema,
  pdf: pdfEditSchema,
};

export function Source({ label, value }: { label: string; value: string }) {
  return (
    <Disclosure title={label}>
      <RecordBody>{value}</RecordBody>
    </Disclosure>
  );
}

export function ProvenanceEditor({
  value,
  change,
  disabled,
  t,
}: {
  value: Provenance;
  change(value: Provenance): void;
  disabled: boolean;
  t: Translate;
}) {
  return (
    <Disclosure title={t("provenance.title")} keepMounted>
      <Metadata>{t("provenance.help")}</Metadata>
      {value.sources.map((source, index) => (
        <RecordGroup key={index}>
          <FormRow>
            <FormField label={t("source.title")}>
              <TextField
                value={source.title}
                maxLength={300}
                disabled={disabled}
                onChange={(event) =>
                  change({
                    ...value,
                    sources: value.sources.map((item, at) =>
                      at === index
                        ? { ...item, title: event.target.value }
                        : item,
                    ),
                  })
                }
              />
            </FormField>
            <FormField label={t("source.uri")}>
              <TextField
                value={source.uri}
                maxLength={2048}
                disabled={disabled}
                onChange={(event) =>
                  change({
                    ...value,
                    sources: value.sources.map((item, at) =>
                      at === index
                        ? { ...item, uri: event.target.value }
                        : item,
                    ),
                  })
                }
              />
            </FormField>
          </FormRow>
          <FormField label={t("source.note")}>
            <TextArea
              value={source.note}
              maxLength={2000}
              disabled={disabled}
              onChange={(event) =>
                change({
                  ...value,
                  sources: value.sources.map((item, at) =>
                    at === index ? { ...item, note: event.target.value } : item,
                  ),
                })
              }
            />
          </FormField>
          <Button
            disabled={disabled}
            onClick={() =>
              change({
                ...value,
                sources: value.sources.filter((_, at) => at !== index),
              })
            }
          >
            {t("source.remove")}
          </Button>
        </RecordGroup>
      ))}
      <Button
        disabled={disabled || value.sources.length >= LIMITS.provenanceSources}
        onClick={() =>
          change({
            ...value,
            sources: [...value.sources, { title: "", uri: "", note: "" }],
          })
        }
      >
        {t("source.add")}
      </Button>
      <FormField label={t("provenance.assumptions")}>
        <TextArea
          value={value.assumptions.join("\n")}
          disabled={disabled}
          onChange={(event) =>
            change({ ...value, assumptions: event.target.value.split("\n") })
          }
        />
      </FormField>
      <FormField label={t("provenance.units")}>
        <TextField
          value={value.units}
          maxLength={2000}
          disabled={disabled}
          onChange={(event) => change({ ...value, units: event.target.value })}
        />
      </FormField>
      <FormField label={t("provenance.reproduction")}>
        <TextArea
          value={value.reproduction}
          maxLength={12000}
          disabled={disabled}
          onChange={(event) =>
            change({ ...value, reproduction: event.target.value })
          }
        />
      </FormField>
    </Disclosure>
  );
}

export function cleanProvenance(value: Provenance): Provenance {
  return {
    ...value,
    assumptions: value.assumptions.filter((item) => item.trim().length),
  };
}

function cell(text: string): CellValue {
  if (text.startsWith("=")) return { type: "formula", value: text };
  if (text === "") return { type: "blank" };
  if (/^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$/u.test(text))
    return { type: "number", value: projectNumber(parseJSON(text)) };
  return { type: "string", value: text };
}

function columnName(index: number): string {
  let name = "";
  for (let value = index + 1; value; value = Math.floor((value - 1) / 26))
    name = String.fromCharCode(65 + ((value - 1) % 26)) + name;
  return name;
}

export function guidedContent(
  format: OfficeFormat,
  title: string,
  body: string,
  sheet: string,
): unknown {
  if (format === "docx")
    return {
      title,
      pageNumbers: true,
      blocks: [
        { type: "paragraph", heading: 1, runs: [{ text: title }] },
        ...body
          .split(/\n\s*\n/u)
          .filter((text) => text.trim())
          .map((text) => ({ type: "paragraph", runs: [{ text }] })),
      ],
    };
  if (format === "xlsx")
    return {
      title,
      sheets: [
        {
          name: sheet,
          freezeRows: 1,
          cells: body.split(/\r?\n/u).flatMap((row, rowIndex) =>
            row.split("\t").map((text, column) => ({
              address: `${columnName(column)}${rowIndex + 1}`,
              value: cell(text),
              ...(rowIndex === 0 ? { style: { bold: true } } : {}),
            })),
          ),
        },
      ],
    };
  const slides: PresentationContent["slides"] = body
    .split(/^\s*---\s*$/mu)
    .filter((text) => text.trim())
    .map((source) => {
      const [heading = "", ...lines] = source.trim().split("\n");
      return {
        title: heading,
        elements: [
          {
            type: "text",
            x: 0.7,
            y: 0.5,
            width: 11.9,
            height: 0.9,
            text: heading,
            fontSize: 30,
            bold: true,
          },
          {
            type: "text",
            x: 0.8,
            y: 1.7,
            width: 11.7,
            height: 4.8,
            text: lines.join("\n"),
            fontSize: 22,
          },
        ],
      };
    });
  return { title, layout: "wide", slides };
}

export function CreateForm({
  state,
  catalog,
  t,
}: {
  state: DeliverablesState;
  catalog?: FormatCatalog;
  t: Translate;
}) {
  const [title, setTitle] = useState("");
  const [format, setFormat] = useState<OfficeFormat>("docx");
  const [mode, setMode] = useState("guided");
  const [bodies, setBodies] = useState<Record<OfficeFormat, string>>({
    docx: "",
    xlsx: "",
    pptx: "",
  });
  const [jsonDrafts, setJSONDrafts] = useState<
    Partial<Record<OfficeFormat, string>>
  >({});
  const [sheet, setSheet] = useState("Sheet1");
  const [provenance, setProvenance] = useState(emptyProvenance);
  const [error, setError] = useState<string>();
  const [created, setCreated] = useState<string>();
  const entry = catalog?.formats.find((item) => item.format === format);
  const json = jsonDrafts[format] ?? entry?.exampleJSON ?? "{}";
  const busy = state.blocked;

  async function submit() {
    if (busy) return;
    setError(undefined);
    setCreated(undefined);
    const selection = state.selection;
    try {
      const content =
        mode === "guided"
          ? guidedContent(format, title, bodies[format], sheet)
          : projectNumbers(parseJSON(json));
      const parsed = creationSchemas[format].safeParse(content);
      if (!parsed.success)
        throw new Error(
          parsed.error.issues
            .map((issue) => `${issue.path.join(".")}: ${issue.message}`)
            .join("\n"),
        );
      const contentJSON = JSON.stringify(parsed.data);
      if (new TextEncoder().encode(contentJSON).length > LIMITS.inputBytes)
        throw new Error(t("author.tooLarge"));
      if (!title.trim()) throw new Error(t("author.titleRequired"));
      await state.run(
        METHODS.create,
        {
          title: title.trim(),
          format,
          contentJSON,
          provenance: cleanProvenance(provenance),
        },
        (revision) => {
          setCreated(revision.id);
          state.selectRevision(revision, selection);
        },
      );
    } catch (cause) {
      if (!state.signal.aborted) setError(failureText(cause));
    }
  }

  return (
    <Form
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      <FormRow>
        <FormField label={t("author.title")}>
          <TextField
            value={title}
            maxLength={180}
            required
            disabled={busy}
            onChange={(event) => setTitle(event.target.value)}
          />
        </FormField>
        <FormGroup label={t("author.format")}>
          <Select
            label={t("author.format")}
            value={format}
            disabled={busy}
            options={(["docx", "xlsx", "pptx"] as const).map((value) => ({
              value,
              label: t(`format.${value}`),
            }))}
            onValueChange={(value) => {
              setFormat(value as OfficeFormat);
              setError(undefined);
            }}
          />
        </FormGroup>
      </FormRow>
      <Select
        label={t("author.mode")}
        value={mode}
        disabled={busy}
        options={[
          { value: "guided", label: t("author.guided") },
          { value: "advanced", label: t("author.advanced") },
        ]}
        onValueChange={setMode}
      />
      {mode === "guided" ? (
        <RecordGroup>
          <Metadata>{t(`author.help.${format}`)}</Metadata>
          {format === "xlsx" && (
            <FormField label={t("author.sheet")}>
              <TextField
                value={sheet}
                maxLength={31}
                disabled={busy}
                onChange={(event) => setSheet(event.target.value)}
              />
            </FormField>
          )}
          <FormField label={t(`author.body.${format}`)}>
            <TextArea
              value={bodies[format]}
              disabled={busy}
              rows={10}
              required
              onChange={(event) =>
                setBodies({ ...bodies, [format]: event.target.value })
              }
            />
          </FormField>
        </RecordGroup>
      ) : (
        <RecordGroup>
          <Metadata>{t("author.advancedHelp")}</Metadata>
          <FormField label={t("author.content")}>
            <TextArea
              font="mono"
              rows={16}
              value={json}
              disabled={busy}
              onChange={(event) =>
                setJSONDrafts({ ...jsonDrafts, [format]: event.target.value })
              }
            />
          </FormField>
          <Toolbar label={t("author.examples")}>
            <Button
              disabled={busy || !entry?.exampleJSON}
              onClick={() =>
                setJSONDrafts({ ...jsonDrafts, [format]: entry?.exampleJSON })
              }
            >
              {t("author.loadExample")}
            </Button>
            <Button
              disabled={busy}
              onClick={() => {
                try {
                  setJSONDrafts({
                    ...jsonDrafts,
                    [format]: JSON.stringify(
                      guidedContent(format, title, bodies[format], sheet),
                      null,
                      2,
                    ),
                  });
                  setError(undefined);
                } catch (cause) {
                  setError(failureText(cause));
                }
              }}
            >
              {t("author.fromGuided")}
            </Button>
          </Toolbar>
          {entry?.createSchemaJSON && (
            <Source label={t("author.schema")} value={entry.createSchemaJSON} />
          )}
        </RecordGroup>
      )}
      <ProvenanceEditor
        value={provenance}
        change={setProvenance}
        disabled={busy}
        t={t}
      />
      <FormActions
        label={t("author.actions")}
        feedback={
          <>
            {error && (
              <Callout kind="warning">
                <RecordBody>{error}</RecordBody>
              </Callout>
            )}
            {created && (
              <Metadata role="status">{t("author.created")}</Metadata>
            )}
          </>
        }
      >
        <Button type="submit" variant="primary" disabled={busy || !entry}>
          {t("author.create")}
        </Button>
      </FormActions>
    </Form>
  );
}

export function RevisionEditor({
  state,
  document,
  inspection,
  catalog,
  t,
}: {
  state: DeliverablesState;
  document: DocumentRead;
  inspection: Inspection;
  catalog?: FormatCatalog;
  t: Translate;
}) {
  const [mode, setMode] = useState("item");
  const [selected, setSelected] = useState("");
  const [text, setText] = useState("");
  const [valueType, setValueType] = useState<CellValue["type"]>("string");
  const [degrees, setDegrees] = useState("90");
  const [operationsJSON, setOperationsJSON] = useState('{"operations":[]}');
  const [error, setError] = useState<string>();
  const item = inspection.items.find((entry) => entry.id === selected);
  const editable = useMemo(
    () =>
      inspection.items.filter((entry) =>
        ["text", "cell", "page", "field"].includes(entry.kind),
      ),
    [inspection],
  );
  const historical = document.document.head !== document.revision.id;
  const disabled = state.blocked || historical;
  const format = document.revision.format;
  const entry = catalog?.formats.find((value) => value.format === format);

  function choose(id: string) {
    const next = editable.find((entry) => entry.id === id);
    setSelected(id);
    setError(undefined);
    if (!next) return;
    if (next.kind === "cell") {
      const parsed = cellValueSchema.safeParse(JSON.parse(next.text));
      if (parsed.success) {
        setValueType(parsed.data.type);
        setText("value" in parsed.data ? String(parsed.data.value) : "");
      } else {
        setValueType("string");
        setText(next.text);
      }
    } else if (next.kind === "field") {
      const source = JSON.parse(next.text) as { value?: unknown };
      setText(JSON.stringify(source.value ?? ""));
    } else setText(next.text);
  }

  async function apply() {
    if (disabled) return;
    setError(undefined);
    const captured = {
      id: document.document.id,
      revision: document.revision.id,
    };
    try {
      let operations: unknown;
      if (mode === "advanced")
        operations = projectNumbers(parseJSON(operationsJSON));
      else if (mode === "append")
        operations = { operations: [{ type: "appendParagraph", text }] };
      else {
        if (!item) throw new Error(t("edit.selectRequired"));
        if (item.kind === "cell") {
          const value: unknown =
            valueType === "blank"
              ? { type: "blank" }
              : {
                  type: valueType,
                  value:
                    valueType === "number"
                      ? projectNumber(parseJSON(text.trim()))
                      : valueType === "boolean"
                        ? text === "true"
                        : text,
                };
          operations = {
            operations: [
              {
                type: "setCell",
                id: item.id,
                expect: item.text,
                value: cellValueSchema.parse(value),
              },
            ],
          };
        } else if (item.kind === "page")
          operations = {
            operations: [
              {
                type: "rotatePage",
                id: item.id,
                expect: item.text,
                degrees: Number(degrees),
              },
            ],
          };
        else if (item.kind === "field")
          operations = {
            operations: [
              {
                type: "fillField",
                id: item.id,
                expect: item.text,
                value: projectNumbers(parseJSON(text)),
              },
            ],
          };
        else
          operations = {
            operations: [
              { type: "replaceText", id: item.id, expect: item.text, text },
            ],
          };
      }
      const parsed = editSchemas[format].safeParse(operations);
      if (!parsed.success)
        throw new Error(
          parsed.error.issues
            .map((issue) => `${issue.path.join(".")}: ${issue.message}`)
            .join("\n"),
        );
      const source = JSON.stringify(parsed.data);
      if (new TextEncoder().encode(source).length > LIMITS.inputBytes)
        throw new Error(t("author.tooLarge"));
      await state.run(
        METHODS.edit,
        {
          id: captured.id,
          expectedRevision: captured.revision,
          operationsJSON: source,
        },
        (revision) => state.selectRevision(revision, captured),
      );
    } catch (cause) {
      if (!state.signal.aborted) setError(failureText(cause));
    }
  }

  return (
    <Disclosure title={t("edit.title")} keepMounted>
      <Metadata>{t("edit.help")}</Metadata>
      {historical && <Callout kind="note">{t("edit.historical")}</Callout>}
      <Form
        onSubmit={(event) => {
          event.preventDefault();
          void apply();
        }}
      >
        <Select
          label={t("edit.mode")}
          value={mode}
          disabled={disabled}
          onValueChange={(value) => {
            setMode(value);
            setError(undefined);
          }}
          options={[
            { value: "item", label: t("edit.itemMode") },
            ...(format === "docx"
              ? [{ value: "append", label: t("edit.append") }]
              : []),
            { value: "advanced", label: t("author.advanced") },
          ]}
        />
        {mode === "advanced" ? (
          <RecordGroup>
            <FormField label={t("edit.operations")}>
              <TextArea
                font="mono"
                rows={12}
                value={operationsJSON}
                disabled={disabled}
                onChange={(event) => setOperationsJSON(event.target.value)}
              />
            </FormField>
            {entry && (
              <Source label={t("edit.schema")} value={entry.editSchemaJSON} />
            )}
          </RecordGroup>
        ) : (
          <RecordGroup>
            {mode === "item" && (
              <Select
                label={t("edit.item")}
                value={selected || null}
                disabled={disabled}
                onValueChange={choose}
                options={editable.map((entry) => ({
                  value: entry.id,
                  label: `${entry.id}: ${entry.text.slice(0, 90)}`,
                }))}
              />
            )}
            {mode === "item" && item && (
              <Source label={t("edit.original")} value={item.text} />
            )}
            {mode === "item" && item?.kind === "cell" && (
              <Select
                label={t("edit.valueType")}
                value={valueType}
                disabled={disabled}
                options={[
                  "string",
                  "number",
                  "boolean",
                  "formula",
                  "blank",
                ].map((value) => ({ value, label: t(`cell.${value}`) }))}
                onValueChange={(value) => {
                  setValueType(value as CellValue["type"]);
                  if (value === "boolean") setText("false");
                }}
              />
            )}
            {mode === "item" && item?.kind === "page" ? (
              <Select
                label={t("edit.rotation")}
                value={degrees}
                disabled={disabled}
                onValueChange={setDegrees}
                options={["0", "90", "180", "270"].map((value) => ({
                  value,
                  label: `${value}°`,
                }))}
              />
            ) : mode === "item" &&
              item?.kind === "cell" &&
              valueType === "boolean" ? (
              <Select
                label={t("edit.value")}
                value={text === "true" ? "true" : "false"}
                disabled={disabled}
                onValueChange={setText}
                options={[
                  { value: "true", label: t("yes") },
                  { value: "false", label: t("no") },
                ]}
              />
            ) : (
              !(
                mode === "item" &&
                item?.kind === "cell" &&
                valueType === "blank"
              ) && (
                <FormField
                  label={t(
                    item?.kind === "field" ? "edit.fieldJSON" : "edit.value",
                  )}
                >
                  <TextArea
                    rows={6}
                    value={text}
                    disabled={disabled}
                    onChange={(event) => setText(event.target.value)}
                  />
                </FormField>
              )
            )}
          </RecordGroup>
        )}
        <FormActions
          label={t("edit.actions")}
          feedback={
            error && (
              <Callout kind="warning">
                <RecordBody>{error}</RecordBody>
              </Callout>
            )
          }
        >
          <Button type="submit" variant="primary" disabled={disabled}>
            {t("edit.apply")}
          </Button>
        </FormActions>
      </Form>
    </Disclosure>
  );
}

import { useMemo, useState } from "react";
import { failureText, type Translate } from "@flame/component-sdk";
import { parseJSON, projectNumbers } from "@frame/sdk/json";
import {
  Button,
  Callout,
  Disclosure,
  Form,
  FormActions,
  FormField,
  Metadata,
  RecordBody,
  RecordGroup,
  Select,
  TextArea,
} from "@flame/ui";
import {
  documentEditSchema,
  workbookEditSchema,
  presentationEditSchema,
  pdfEditSchema,
  PAGE_ROTATIONS,
  type FormatCatalog,
  type Inspection,
  type CellValue,
} from "../contracts/content";
import { LIMITS } from "../contracts/limits";
import { METHODS } from "../contracts/methods";
import { type DocumentRead } from "./types";
import {
  type AuthoringState,
  EDITABLE_KINDS,
  type EditMode,
  type InspectionItem,
  type ItemDraft,
  itemDraftText,
  itemOperation,
} from "./authoringItems";
import { Source } from "./provenance";

const editSchemas = {
  docx: documentEditSchema,
  xlsx: workbookEditSchema,
  pptx: presentationEditSchema,
  pdf: pdfEditSchema,
};

export function RevisionEditor({
  state,
  document,
  inspection,
  catalog,
  t,
}: {
  state: AuthoringState;
  document: DocumentRead;
  inspection: Inspection;
  catalog?: FormatCatalog;
  t: Translate;
}) {
  const [mode, setMode] = useState<EditMode>("item");
  const [selected, setSelected] = useState("");
  const [text, setText] = useState("");
  const [valueType, setValueType] = useState<CellValue["type"]>("string");
  const [degrees, setDegrees] = useState("90");
  const [operationsJSON, setOperationsJSON] = useState('{"operations":[]}');
  const [error, setError] = useState<string>();
  const editable = useMemo(
    () => inspection.items.filter((entry) => EDITABLE_KINDS.has(entry.kind)),
    [inspection],
  );
  const target =
    mode === "item"
      ? inspection.items.find((entry) => entry.id === selected)
      : undefined;
  const historical = document.document.head !== document.revision.id;
  const disabled = state.blocked || historical;
  const format = document.revision.format;
  const entry = catalog?.formats.find((value) => value.format === format);

  function choose(id: string) {
    const next = editable.find((entry) => entry.id === id);
    setSelected(id);
    setError(undefined);
    if (!next) return;
    const draft = itemDraftText(next);
    setText(draft.text);
    if (draft.valueType) setValueType(draft.valueType);
  }

  function operations(): unknown {
    if (mode === "advanced") return projectNumbers(parseJSON(operationsJSON));
    if (mode === "append")
      return { operations: [{ type: "appendParagraph", text }] };
    if (!target) throw new Error(t("edit.selectRequired"));
    return {
      operations: [itemOperation(target, { text, valueType, degrees })],
    };
  }

  async function apply() {
    if (disabled) return;
    setError(undefined);
    const captured = {
      id: document.document.id,
      revision: document.revision.id,
    };
    try {
      const parsed = editSchemas[format].safeParse(operations());
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
            setMode(value as EditMode);
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
            {target && (
              <Source label={t("edit.original")} value={target.text} />
            )}
            {target?.kind === "cell" && (
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
            <ItemValueField
              target={target}
              draft={{ text, valueType, degrees }}
              disabled={disabled}
              onText={setText}
              onDegrees={setDegrees}
              t={t}
            />
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

// Appending a paragraph has no target and edits plain text.
function ItemValueField({
  target,
  draft,
  disabled,
  onText,
  onDegrees,
  t,
}: {
  target: InspectionItem | undefined;
  draft: ItemDraft;
  disabled: boolean;
  onText(text: string): void;
  onDegrees(degrees: string): void;
  t: Translate;
}) {
  if (target?.kind === "page")
    return (
      <Select
        label={t("edit.rotation")}
        value={draft.degrees}
        disabled={disabled}
        onValueChange={onDegrees}
        options={PAGE_ROTATIONS.map((value) => ({
          value: String(value),
          label: `${value}°`,
        }))}
      />
    );
  if (target?.kind === "cell" && draft.valueType === "blank") return null;
  if (target?.kind === "cell" && draft.valueType === "boolean")
    return (
      <Select
        label={t("edit.value")}
        value={draft.text === "true" ? "true" : "false"}
        disabled={disabled}
        onValueChange={onText}
        options={[
          { value: "true", label: t("yes") },
          { value: "false", label: t("no") },
        ]}
      />
    );
  return (
    <FormField
      label={t(target?.kind === "field" ? "edit.fieldJSON" : "edit.value")}
    >
      <TextArea
        rows={6}
        value={draft.text}
        disabled={disabled}
        onChange={(event) => onText(event.target.value)}
      />
    </FormField>
  );
}

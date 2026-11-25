import { useState } from "react";
import { failureText, type Translate } from "@flame/component-sdk";
import { parseJSON, projectNumbers } from "@frame/sdk/json";
import {
  Button,
  Callout,
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
  OFFICE_FORMATS,
  officeFormatSchema,
  type FormatCatalog,
  type OfficeFormat,
  type PresentationContent,
} from "../contracts/content";
import { FIELD_LIMITS, LIMITS } from "../contracts/limits";
import { METHODS } from "../contracts/methods";
import { emptyProvenance } from "./types";
import { type AuthoringState, cell, columnName } from "./authoringItems";
import { ProvenanceEditor, Source, cleanProvenance } from "./provenance";

const creationSchemas = {
  docx: documentContentSchema,
  xlsx: workbookContentSchema,
  pptx: presentationContentSchema,
};

// Guided authoring fills a format's layout; advanced authoring takes raw content JSON.
const AUTHORING_MODES = ["guided", "advanced"] as const;
type AuthoringMode = (typeof AUTHORING_MODES)[number];
function guidedContent(
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
  state: AuthoringState;
  catalog?: FormatCatalog;
  t: Translate;
}) {
  const [title, setTitle] = useState("");
  const [format, setFormat] = useState<OfficeFormat>("docx");
  const [mode, setMode] = useState<AuthoringMode>("guided");
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
            maxLength={FIELD_LIMITS.title}
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
            options={OFFICE_FORMATS.map((value) => ({
              value,
              label: t(`format.${value}`),
            }))}
            onValueChange={(value) => {
              setFormat(officeFormatSchema.parse(value));
              setError(undefined);
            }}
          />
        </FormGroup>
      </FormRow>
      <Select
        label={t("author.mode")}
        value={mode}
        disabled={busy}
        options={AUTHORING_MODES.map((value) => ({
          value,
          label: t(`author.${value}`),
        }))}
        onValueChange={setMode}
      />
      {mode === "guided" ? (
        <RecordGroup>
          <Metadata>{t(`author.help.${format}`)}</Metadata>
          {format === "xlsx" && (
            <FormField label={t("author.sheet")}>
              <TextField
                value={sheet}
                maxLength={FIELD_LIMITS.sheetName}
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

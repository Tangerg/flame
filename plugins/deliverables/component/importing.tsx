import { useEffect, useRef, useState } from "react";
import { failureText, type Translate } from "@flame/component-sdk";
import {
  Button,
  Callout,
  Facts,
  Form,
  FormActions,
  FormField,
  FormGroup,
  FormRow,
  Metadata,
  RecordBody,
  RecordGroup,
  Select,
  TextField,
  Toolbar,
} from "@flame/ui";
import { FORMATS, formatSchema, type Format } from "../contracts/content";
import { FIELD_LIMITS, LIMITS } from "../contracts/limits";
import { METHODS } from "../contracts/methods";
import { cleanProvenance, ProvenanceEditor } from "./authoring";
import { boundedInteger, encodeChunk, sha256 } from "./transfer";
import { emptyProvenance, type Upload } from "./types";
import type { DeliverablesState } from "./useDeliverables";

const ACCEPTED_FILES = FORMATS.map((format) => `.${format}`).join(",");

type FileSelection =
  | { kind: "empty" }
  | { kind: "hashing" }
  | { kind: "chosen"; file: File; sha256: string }
  | { kind: "rejected"; error: string };

type Submission =
  | { kind: "idle" }
  | { kind: "transferring"; progress?: { received: number; total: number } }
  | { kind: "finished" }
  | { kind: "failed"; error: string };

export function ImportForm({
  state,
  t,
}: {
  state: DeliverablesState;
  t: Translate;
}) {
  const [title, setTitle] = useState("");
  const [format, setFormat] = useState<Format>("docx");
  const [provenance, setProvenance] = useState(emptyProvenance);
  const [selection, setSelection] = useState<FileSelection>({
    kind: "empty",
  });
  const [submission, setSubmission] = useState<Submission>({ kind: "idle" });
  const [uploadId, setUploadId] = useState<string>();
  const hashingRequest = useRef<AbortController>(undefined);
  useEffect(() => () => hashingRequest.current?.abort(), []);
  const uploads = state.uploads?.value ?? [];
  const transferring = submission.kind === "transferring";
  const blocked = state.blocked || transferring || selection.kind === "hashing";
  const selected = uploads.find((upload) => upload.id === uploadId);
  const error =
    selection.kind === "rejected"
      ? selection.error
      : submission.kind === "failed"
        ? submission.error
        : undefined;

  async function choose(file?: File) {
    hashingRequest.current?.abort();
    const request = new AbortController();
    hashingRequest.current = request;
    setSubmission({ kind: "idle" });
    if (!file) {
      setSelection({ kind: "empty" });
      return;
    }
    if (!file.size || file.size > LIMITS.fileBytes) {
      setSelection({
        kind: "rejected",
        error: t("import.limit", { maximum: LIMITS.fileBytes / 1024 / 1024 }),
      });
      return;
    }
    const signal = AbortSignal.any([state.signal, request.signal]);
    setSelection({ kind: "hashing" });
    try {
      const digest = await sha256(new Uint8Array(await file.arrayBuffer()));
      if (signal.aborted) return;
      setSelection({ kind: "chosen", file, sha256: digest });
      const extension = formatSchema.safeParse(
        file.name.split(".").at(-1)?.toLowerCase(),
      );
      if (extension.success) setFormat(extension.data);
      setTitle(
        (previous) =>
          previous ||
          file.name.replace(/\.[^.]+$/u, "").slice(0, FIELD_LIMITS.title),
      );
    } catch (cause) {
      if (!signal.aborted)
        setSelection({ kind: "rejected", error: failureText(cause) });
    }
  }

  async function submit() {
    if (blocked || selection.kind !== "chosen") return;
    const captured = {
      local: selection,
      title: title.trim(),
      format,
      provenance: cleanProvenance(provenance),
      uploadId,
      selection: state.selection,
    };
    if (!captured.title) {
      setSubmission({ kind: "failed", error: t("author.titleRequired") });
      return;
    }
    setSubmission({ kind: "transferring" });
    try {
      const current = await state.refreshUploads();
      state.signal.throwIfAborted();
      if (!current) throw new Error(t("import.readRequired"));
      let upload: Upload | undefined;
      if (captured.uploadId) {
        upload = current.find((value) => value.id === captured.uploadId);
        if (!upload) throw new Error(t("import.missing"));
      } else {
        upload = await state.run(
          METHODS.upload,
          { size: captured.local.file.size, sha256: captured.local.sha256 },
          (value) => setUploadId(value.id),
        );
        if (!upload) return;
      }
      if (
        upload.sha256 !== captured.local.sha256 ||
        boundedInteger(upload.size, LIMITS.fileBytes) !==
          captured.local.file.size
      )
        throw new Error(t("import.mismatch"));
      let offset = boundedInteger(upload.received, captured.local.file.size);
      setSubmission({
        kind: "transferring",
        progress: { received: offset, total: captured.local.file.size },
      });
      while (offset < captured.local.file.size) {
        state.signal.throwIfAborted();
        const bytes = new Uint8Array(
          await captured.local.file
            .slice(offset, offset + LIMITS.chunkBytes)
            .arrayBuffer(),
        );
        state.signal.throwIfAborted();
        const result = await state.run(
          METHODS.append,
          { uploadId: upload.id, offset, base64: encodeChunk(bytes) },
          (value) => {
            setSubmission({
              kind: "transferring",
              progress: {
                received: boundedInteger(
                  value.received,
                  captured.local.file.size,
                ),
                total: captured.local.file.size,
              },
            });
          },
        );
        if (!result) return;
        const next = boundedInteger(result.received, captured.local.file.size);
        if (
          result.id !== upload.id ||
          result.sha256 !== captured.local.sha256 ||
          next !== offset + bytes.length
        )
          throw new Error(
            "deliverables: upload acknowledgement differs from its captured bytes",
          );
        offset = next;
      }
      await state.run(
        METHODS.import,
        {
          title: captured.title,
          format: captured.format,
          uploadId: upload.id,
          provenance: captured.provenance,
        },
        (revision) => {
          setUploadId(undefined);
          setSubmission({ kind: "finished" });
          state.selectRevision(revision, captured.selection);
        },
      );
    } catch (cause) {
      if (!state.signal.aborted)
        setSubmission({ kind: "failed", error: failureText(cause) });
    } finally {
      if (!state.signal.aborted) {
        setSubmission((current) =>
          current.kind === "transferring" ? { kind: "idle" } : current,
        );
        void state.refreshUploads();
      }
    }
  }

  return (
    <RecordGroup>
      <Metadata>{t("import.help")}</Metadata>
      <Form
        onSubmit={(event) => {
          event.preventDefault();
          void submit();
        }}
      >
        <FormField label={t("import.file")}>
          <input
            type="file"
            accept={ACCEPTED_FILES}
            disabled={blocked}
            onChange={(event) => {
              void choose(event.target.files?.[0]);
            }}
          />
        </FormField>
        <FormRow>
          <FormField label={t("author.title")}>
            <TextField
              value={title}
              required
              maxLength={FIELD_LIMITS.title}
              disabled={blocked}
              onChange={(event) => setTitle(event.target.value)}
            />
          </FormField>
          <FormGroup label={t("author.format")}>
            <Select
              label={t("author.format")}
              value={format}
              disabled={blocked}
              options={FORMATS.map((value) => ({
                value,
                label: t(`format.${value}`),
              }))}
              onValueChange={(value) => setFormat(formatSchema.parse(value))}
            />
          </FormGroup>
        </FormRow>
        {selection.kind === "hashing" && (
          <Metadata role="status">{t("import.hashing")}</Metadata>
        )}
        {selection.kind === "chosen" && (
          <Facts
            values={[
              [t("import.filename"), selection.file.name],
              [t("file.size"), String(selection.file.size)],
              [t("file.hash"), selection.sha256],
            ]}
          />
        )}
        {selected && (
          <Callout kind="note">
            {t("import.resuming", {
              id: selected.id.slice(0, 12),
              received: String(selected.received),
            })}
          </Callout>
        )}
        <ProvenanceEditor
          value={provenance}
          change={setProvenance}
          disabled={blocked}
          t={t}
        />
        <FormActions
          label={t("import.actions")}
          feedback={
            <>
              {error && (
                <Callout kind="warning">
                  <RecordBody>{error}</RecordBody>
                </Callout>
              )}
              {submission.kind === "transferring" && submission.progress && (
                <Metadata role="status">
                  {t("transfer.progress", {
                    received: submission.progress.received,
                    total: submission.progress.total,
                  })}
                </Metadata>
              )}
              {submission.kind === "finished" && (
                <Metadata role="status">{t("import.finished")}</Metadata>
              )}
            </>
          }
        >
          {uploadId && (
            <Button disabled={blocked} onClick={() => setUploadId(undefined)}>
              {t("import.new")}
            </Button>
          )}
          <Button
            type="submit"
            variant="primary"
            disabled={blocked || selection.kind !== "chosen"}
          >
            {t(uploadId ? "import.resume" : "import.submit")}
          </Button>
        </FormActions>
      </Form>
      {state.uploads?.error && (
        <Callout kind="warning">{state.uploads.error}</Callout>
      )}
      {uploads.length > 0 && (
        <RecordGroup aria-label={t("import.staged")}>
          <Metadata>{t("import.stagedHelp")}</Metadata>
          {uploads.map((upload) => (
            <RecordGroup key={upload.id}>
              <Facts
                values={[
                  [t("import.stagedId"), upload.id],
                  [t("file.hash"), upload.sha256],
                  [
                    t("transfer.bytes"),
                    `${String(upload.received)} / ${String(upload.size)}`,
                  ],
                ]}
              />
              <Toolbar label={t("import.stagedActions")}>
                <Button
                  disabled={blocked}
                  aria-pressed={uploadId === upload.id}
                  onClick={() => {
                    setUploadId(upload.id);
                    setSubmission({ kind: "idle" });
                  }}
                >
                  {t("import.select")}
                </Button>
                <Button
                  disabled={
                    transferring ||
                    state.commands.some(
                      (command) => command.method === METHODS.discard,
                    )
                  }
                  onClick={() => {
                    const captured = upload.id;
                    void state.run(
                      METHODS.discard,
                      { uploadId: captured },
                      () =>
                        setUploadId((current) =>
                          current === captured ? undefined : current,
                        ),
                    );
                  }}
                >
                  {t("import.discard")}
                </Button>
              </Toolbar>
            </RecordGroup>
          ))}
        </RecordGroup>
      )}
    </RecordGroup>
  );
}

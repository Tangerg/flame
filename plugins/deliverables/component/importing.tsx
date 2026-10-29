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
import { LIMITS } from "../contracts/limits";
import { METHODS } from "../contracts/methods";
import { cleanProvenance, ProvenanceEditor } from "./authoring";
import { boundedInteger, encodeChunk, sha256 } from "./transfer";
import { emptyProvenance, type Format, type Upload } from "./types";
import type { DeliverablesState } from "./useDeliverables";

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
  const [local, setLocal] = useState<{ file: File; sha256: string }>();
  const [hashing, setHashing] = useState(false);
  const [working, setWorking] = useState(false);
  const [uploadId, setUploadId] = useState<string>();
  const [progress, setProgress] = useState<{
    received: number;
    total: number;
  }>();
  const [error, setError] = useState<string>();
  const [finished, setFinished] = useState(false);
  const hashingRequest = useRef<AbortController>(undefined);
  const active = useRef(false);
  useEffect(() => () => hashingRequest.current?.abort(), []);
  const uploads = state.uploads?.value ?? [];
  const blocked = state.blocked || working || hashing;
  const selected = uploads.find((upload) => upload.id === uploadId);

  async function choose(file?: File) {
    hashingRequest.current?.abort();
    const request = new AbortController();
    hashingRequest.current = request;
    setLocal(undefined);
    setError(undefined);
    setFinished(false);
    setHashing(false);
    if (!file) return;
    const signal = AbortSignal.any([state.signal, request.signal]);
    try {
      if (!file.size || file.size > LIMITS.fileBytes)
        throw new Error(
          t("import.limit", { maximum: LIMITS.fileBytes / 1024 / 1024 }),
        );
      setHashing(true);
      const digest = await sha256(new Uint8Array(await file.arrayBuffer()));
      if (signal.aborted) return;
      setLocal({ file, sha256: digest });
      const extension = file.name.split(".").at(-1)?.toLowerCase();
      if (["docx", "xlsx", "pptx", "pdf"].includes(extension ?? ""))
        setFormat(extension as Format);
      setTitle(
        (previous) =>
          previous || file.name.replace(/\.[^.]+$/u, "").slice(0, 180),
      );
    } catch (cause) {
      if (!signal.aborted) setError(failureText(cause));
    } finally {
      if (!signal.aborted) setHashing(false);
    }
  }

  async function submit() {
    if (blocked || active.current || !local) return;
    const captured = {
      local,
      title: title.trim(),
      format,
      provenance: cleanProvenance(provenance),
      uploadId,
      selection: state.selection,
    };
    if (!captured.title) {
      setError(t("author.titleRequired"));
      return;
    }
    active.current = true;
    setWorking(true);
    setFinished(false);
    setError(undefined);
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
      setProgress({ received: offset, total: captured.local.file.size });
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
            setProgress({
              received: boundedInteger(
                value.received,
                captured.local.file.size,
              ),
              total: captured.local.file.size,
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
          setProgress(undefined);
          setFinished(true);
          state.selectRevision(revision, captured.selection);
        },
      );
    } catch (cause) {
      if (!state.signal.aborted) setError(failureText(cause));
    } finally {
      active.current = false;
      if (!state.signal.aborted) {
        setWorking(false);
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
            accept=".docx,.xlsx,.pptx,.pdf"
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
              maxLength={180}
              disabled={blocked}
              onChange={(event) => setTitle(event.target.value)}
            />
          </FormField>
          <FormGroup label={t("author.format")}>
            <Select
              label={t("author.format")}
              value={format}
              disabled={blocked}
              options={["docx", "xlsx", "pptx", "pdf"].map((value) => ({
                value,
                label: t(`format.${value}`),
              }))}
              onValueChange={(value) => setFormat(value as Format)}
            />
          </FormGroup>
        </FormRow>
        {hashing && <Metadata role="status">{t("import.hashing")}</Metadata>}
        {local && (
          <Facts
            values={[
              [t("import.filename"), local.file.name],
              [t("file.size"), String(local.file.size)],
              [t("file.hash"), local.sha256],
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
              {progress && (
                <Metadata role="status">
                  {t("transfer.progress", {
                    received: progress.received,
                    total: progress.total,
                  })}
                </Metadata>
              )}
              {finished && (
                <Metadata role="status">{t("import.finished")}</Metadata>
              )}
            </>
          }
        >
          {uploadId && (
            <Button
              disabled={blocked}
              onClick={() => {
                setUploadId(undefined);
                setProgress(undefined);
              }}
            >
              {t("import.new")}
            </Button>
          )}
          <Button type="submit" variant="primary" disabled={blocked || !local}>
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
                    setError(undefined);
                    setProgress(undefined);
                  }}
                >
                  {t("import.select")}
                </Button>
                <Button
                  disabled={
                    working ||
                    state.commands.some(
                      (command) => command.method === METHODS.discard,
                    )
                  }
                  onClick={() => {
                    const captured = upload.id;
                    void state.run(
                      METHODS.discard,
                      { uploadId: captured },
                      () => {
                        setUploadId((current) =>
                          current === captured ? undefined : current,
                        );
                        setProgress(undefined);
                      },
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

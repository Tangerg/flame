import { useEffect, useMemo, useState } from "react";
import {
  bindOperations,
  failureText,
  mutationSettlementIsUnknown,
  useTranslator,
  type ComponentContext,
} from "@flame/component-sdk";
import { projectNumber } from "@frame/sdk/json";
import type { MutationPromise, Project } from "@flame/runtime-client";
import {
  Button,
  FormActions,
  FormField,
  Metadata,
  Panel,
  PanelSection,
  PanelTitle,
  RecordBody,
  RecordGroup,
  Select,
  TextArea,
  TextField,
} from "@flame/ui";
import {
  CAPABILITY_JSON,
  type MethodInputs,
  type MethodOutputs,
} from "../package/io.github.tangerg.flame/contracts/methods.generated";
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { de } from "./locales/de";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";

const catalogs = { en, zh, "zh-TW": zhTW, de, es, fr, ja, ko };
const library = "flame.knowledgebase/list.v1";
const move = "flame.knowledgebase/move.v1";
const importSource = "flame.knowledgebase/import.v1";
const read = "flame.knowledgebase/read.v1";
const claims = "flame.knowledgebase/list-claims.v1";
const readClaim = "flame.knowledgebase/read-claim.v1";
const accept = "flame.knowledgebase/accept-claim.v1";
type Citation = MethodInputs[typeof accept]["citations"][number];
type Target = {
  project: string;
  material?: NonNullable<MethodInputs[typeof read]>["id"];
  claim?: string;
  revision?: number;
  page?: number;
};
type Messages = ReturnType<typeof useTranslator>;

function navigationTarget(value: string): Target | undefined {
  try {
    const item = JSON.parse(value);
    if (typeof item?.project !== "string" || !item.project) return;
    const number = (n: unknown) =>
      typeof n === "number" && Number.isSafeInteger(n) && n > 0 ? n : undefined;
    return {
      project: item.project,
      material: typeof item.material === "string" ? item.material : undefined,
      claim: typeof item.claim === "string" ? item.claim : undefined,
      revision: number(item.revision),
      page: number(item.page),
    };
  } catch (error) {
    if (!(error instanceof SyntaxError)) throw error;
  }
}

export function ProjectResearch({ context }: { context: ComponentContext }) {
  const t = useTranslator(catalogs, context.locale);
  const [target, setTarget] = useState<Target>();
  const projectId = target?.project ?? context.project?.id;
  const [read, setRead] = useState<{
    id: string;
    project?: Project;
    error?: string;
  }>();
  useEffect(
    () =>
      context.views?.subscribeTarget((value) => {
        const next = navigationTarget(value);
        if (next) setTarget(next);
      }),
    [context.views],
  );
  useEffect(() => {
    if (!projectId) return;
    const reader = new AbortController();
    void context
      .track(() =>
        context.runtime.projects.get(
          projectId,
          AbortSignal.any([reader.signal, context.signal]),
        ),
      )
      .then(
        (project) => {
          if (!reader.signal.aborted) setRead({ id: projectId, project });
        },
        (error) => {
          if (!reader.signal.aborted)
            setRead({ id: projectId, error: failureText(error) });
        },
      );
    return () => reader.abort();
  }, [context.runtime, context.signal, context.track, projectId]);
  const project = read?.id === projectId ? read?.project : undefined;

  return (
    <Panel label={t("title")} header={<PanelTitle>{t("title")}</PanelTitle>}>
      {project ? (
        <Research
          key={JSON.stringify([project.id, target])}
          context={context}
          project={project}
          target={target}
          t={t}
        />
      ) : read?.id === projectId && read?.error ? (
        <p role="alert">{read.error}</p>
      ) : (
        <Metadata>{t(projectId ? "loading" : "chooseProject")}</Metadata>
      )}
    </Panel>
  );
}

function Research({
  context,
  project,
  target,
  t,
}: {
  context: ComponentContext;
  project: Project;
  target?: Target;
  t: Messages;
}) {
  const operations = useMemo(() => {
    if (!context.binding)
      throw new Error("Project Research requires its own contribution");
    return bindOperations<MethodInputs, MethodOutputs>(
      context,
      CAPABILITY_JSON,
      { scope: "installation", reference: context.binding.installationId },
    );
  }, [context]);
  const scope = useMemo(() => ({ project: project.id }), [project.id]);
  const [sources, setSources] = useState<MethodOutputs[typeof library]>([]);
  const [results, setResults] = useState<MethodOutputs[typeof claims]>([]);
  const [material, setMaterial] = useState<
    MethodInputs[typeof read] | undefined
  >(
    target?.material
      ? {
          ...scope,
          id: target.material,
          revision: target.revision,
          page: target.page ?? 1,
        }
      : undefined,
  );
  const [opened, setOpened] = useState<
    MethodInputs[typeof readClaim] | undefined
  >(
    target?.claim
      ? { ...scope, id: target.claim, revision: target.revision }
      : undefined,
  );
  const [document, setDocument] = useState<MethodOutputs[typeof read]>();
  const [claim, setClaim] = useState<MethodOutputs[typeof readClaim]>();
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [refresh, setRefresh] = useState(0);
  const [retry, setRetry] = useState<() => void>();
  const [file, setFile] = useState<File>();
  const [title, setTitle] = useState("");
  const [statement, setStatement] = useState("");
  const [reason, setReason] = useState("");
  const [citations, setCitations] = useState<Citation[]>([]);
  const [editing, setEditing] = useState<{ id: string; revision: number }>();
  const [first, setFirst] = useState("1");
  const [last, setLast] = useState("1");

  useEffect(() => {
    const reader = new AbortController();
    void Promise.all([
      operations.query(library, scope, reader.signal),
      operations.query(claims, scope, reader.signal),
    ]).then(
      ([nextSources, nextResults]) => {
        if (reader.signal.aborted) return;
        setSources(nextSources);
        setResults(nextResults);
      },
      (failure) => {
        if (!reader.signal.aborted) setError(failureText(failure));
      },
    );
    return () => reader.abort();
  }, [operations, scope, refresh]);
  useEffect(() => {
    if (!material) return;
    const reader = new AbortController();
    setDocument(undefined);
    void operations.query(read, material, reader.signal).then(
      (next) => {
        if (!reader.signal.aborted) setDocument(next);
      },
      (failure) => {
        if (!reader.signal.aborted) setError(failureText(failure));
      },
    );
    return () => reader.abort();
  }, [operations, material, refresh]);
  useEffect(() => {
    if (!opened) return;
    const reader = new AbortController();
    setClaim(undefined);
    void operations.query(readClaim, opened, reader.signal).then(
      (next) => {
        if (!reader.signal.aborted) setClaim(next);
      },
      (failure) => {
        if (!reader.signal.aborted) setError(failureText(failure));
      },
    );
    return () => reader.abort();
  }, [operations, opened, refresh]);

  async function settle<T>(
    pending: MutationPromise<T>,
    onSuccess: (reply: T) => void,
  ) {
    setBusy(true);
    setRetry(undefined);
    setError(undefined);
    try {
      onSuccess(await pending);
      setRefresh((current) => current + 1);
    } catch (failure) {
      setError(failureText(failure));
      if (mutationSettlementIsUnknown(failure))
        setRetry(() => () => {
          void settle(pending.retry(), onSuccess);
        });
    } finally {
      setBusy(false);
    }
  }
  const addFile = async (replace: boolean) => {
    if (!file || busy || retry) return;
    setBusy(true);
    setError(undefined);
    try {
      if (file.size > 65_536) throw new Error(t("tooLarge"));
      const text = new TextDecoder("utf-8", { fatal: true }).decode(
        await file.arrayBuffer(),
      );
      const previous =
        replace && document?.document.project === project.id
          ? document.document
          : undefined;
      if (replace && !previous) return;
      await settle(
        operations.command(importSource, {
          content: {
            ...scope,
            id: previous?.id ?? crypto.randomUUID(),
            title: title.trim() || previous?.title || file.name,
            source: file.name,
            text,
          },
          expectedRevision: previous?.revision ?? 0,
        }),
        (header) => {
          setFile(undefined);
          setTitle("");
          setMaterial({
            ...scope,
            id: header.id,
            revision: header.revision,
            page: 1,
          });
        },
      );
    } catch (failure) {
      setError(failureText(failure));
    } finally {
      setBusy(false);
    }
  };
  const page = document?.page === undefined ? 1 : projectNumber(document.page);
  const lines = document?.text.split("\n") ?? [];
  const start = Number(first),
    end = Number(last);
  const validLines =
    Number.isSafeInteger(start) &&
    Number.isSafeInteger(end) &&
    start >= 1 &&
    end >= start &&
    end <= lines.length;
  const mutable = busy || !!retry || project.archived === true;
  const exactLink = (value: Target) =>
    context.views?.href(context.component.componentId, JSON.stringify(value));

  return (
    <>
      {error && <p role="alert">{error}</p>}
      <PanelSection title={project.name}>
        <FormActions label={t("title")}>
          <Button
            disabled={busy}
            onClick={() => setRefresh((current) => current + 1)}
          >
            {t("refresh")}
          </Button>
          {retry && (
            <Button disabled={busy} onClick={retry}>
              {t("recover")}
            </Button>
          )}
        </FormActions>
      </PanelSection>
      <PanelSection title={t("materials")}>
        <RecordGroup>
          {sources.map((source) => (
            <Button
              key={source.id}
              onClick={() => {
                setError(undefined);
                setMaterial({ ...scope, id: source.id, page: 1 });
              }}
            >
              {source.title} · {source.source}
            </Button>
          ))}
        </RecordGroup>
        <FormField label={t("file")}>
          <input
            type="file"
            accept=".txt,.md,text/plain,text/markdown"
            disabled={mutable}
            onChange={(event) => setFile(event.target.files?.[0])}
          />
        </FormField>
        <FormField label={t("materialTitle")}>
          <TextField
            value={title}
            onChange={(event) => setTitle(event.target.value)}
          />
        </FormField>
        <FormActions label={t("materials")}>
          <Button
            disabled={mutable || !file}
            onClick={() => void addFile(false)}
          >
            {t("import")}
          </Button>
          {document?.document.project === project.id && (
            <Button
              disabled={mutable || !file}
              onClick={() => void addFile(true)}
            >
              {t("replace")}
            </Button>
          )}
        </FormActions>
        {document && (
          <RecordGroup>
            <Metadata>
              {document.document.title} ·{" "}
              {t("page", { page, total: projectNumber(document.pages) })}
            </Metadata>
            {document.document.project === project.id && (
              <MoveMaterial
                key={document.document.id}
                context={context}
                project={project.id}
                t={t}
                disabled={mutable}
                onMove={(targetProject) => {
                  void settle(
                    operations.command(move, {
                      ...scope,
                      id: document.document.id,
                      expectedRevision: document.document.revision,
                      targetProject,
                    }),
                    () => {
                      setMaterial(undefined);
                      setDocument(undefined);
                      setCitations((existing) =>
                        existing.filter(
                          (item) => item.documentId !== document.document.id,
                        ),
                      );
                    },
                  );
                }}
              />
            )}
            <ol style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}>
              {lines.map((line, index) => (
                <li key={index}>{line}</li>
              ))}
            </ol>
            <FormActions label={t("materials")}>
              {[-1, 1].map((delta) => (
                <Button
                  key={delta}
                  disabled={
                    page + delta < 1 ||
                    page + delta > projectNumber(document.pages)
                  }
                  onClick={() =>
                    setMaterial({
                      ...scope,
                      id: document.document.id,
                      revision: document.document.revision,
                      page: page + delta,
                    })
                  }
                >
                  {t(delta === -1 ? "previous" : "next")}
                </Button>
              ))}
            </FormActions>
            <FormField label={t("first")}>
              <TextField
                inputMode="numeric"
                value={first}
                onChange={(e) => setFirst(e.target.value)}
              />
            </FormField>
            <FormField label={t("last")}>
              <TextField
                inputMode="numeric"
                value={last}
                onChange={(e) => setLast(e.target.value)}
              />
            </FormField>
            <Button
              disabled={!validLines || citations.length >= 8 || mutable}
              onClick={() =>
                setCitations((existing) => [
                  ...existing,
                  {
                    documentId: document.document.id,
                    revision: document.document.revision,
                    page,
                    firstLine: start,
                    lastLine: end,
                  },
                ])
              }
            >
              {t("cite")}
            </Button>
          </RecordGroup>
        )}
      </PanelSection>
      <PanelSection title={t("claims")}>
        <RecordGroup>
          {results.map((item) => (
            <Button
              key={item.id}
              onClick={() => {
                setError(undefined);
                setOpened({ ...scope, id: item.id });
              }}
            >
              {item.statement ?? item.id} · {t(item.state)}
            </Button>
          ))}
        </RecordGroup>
        {claim && (
          <RecordGroup>
            <Metadata>
              {t(claim.state)} · v{String(claim.revision)}
            </Metadata>
            {claim.statement && <RecordBody>{claim.statement}</RecordBody>}
            {claim.reason && <RecordBody>{claim.reason}</RecordBody>}
            {claim.evidence.map((item, index) => (
              <RecordGroup key={index}>
                <Button
                  onClick={() =>
                    setMaterial({
                      ...scope,
                      id: item.documentId,
                      revision: item.revision,
                      page: item.page,
                    })
                  }
                >
                  {item.title} · {item.source} · {String(item.page)}:
                  {String(item.firstLine)}–{String(item.lastLine)}
                </Button>
                <RecordBody>{item.excerpt}</RecordBody>
                {item.needsRecheck && <Metadata>{t("recheck")}</Metadata>}
              </RecordGroup>
            ))}
            {context.views && (
              <a
                href={exactLink({
                  ...scope,
                  claim: claim.id,
                  revision: projectNumber(claim.revision),
                })}
              >
                {t("link")}
              </a>
            )}
            {claim.statement &&
              claim.reason &&
              results.some(
                (item) =>
                  item.id === claim.id &&
                  String(item.revision) === String(claim.revision),
              ) && (
                <Button
                  disabled={mutable}
                  onClick={() => {
                    setEditing({
                      id: claim.id,
                      revision: projectNumber(claim.revision),
                    });
                    setStatement(claim.statement ?? "");
                    setReason(claim.reason ?? "");
                    setCitations(
                      claim.evidence
                        .filter((item) => !item.needsRecheck)
                        .map(
                          ({
                            documentId,
                            revision,
                            page,
                            firstLine,
                            lastLine,
                          }) => ({
                            documentId,
                            revision,
                            page,
                            firstLine,
                            lastLine,
                          }),
                        ),
                    );
                  }}
                >
                  {t("revise")}
                </Button>
              )}
          </RecordGroup>
        )}
        <FormField label={t("statement")}>
          <TextArea
            autosize
            value={statement}
            onChange={(e) => setStatement(e.target.value)}
          />
        </FormField>
        <FormField label={t("reason")}>
          <TextArea
            autosize
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
        </FormField>
        <RecordGroup>
          {citations.map((item, index) => (
            <Button
              key={index}
              onClick={() =>
                setCitations((existing) =>
                  existing.filter((_, i) => i !== index),
                )
              }
            >
              {t("remove")} · {item.documentId} · v{String(item.revision)} ·{" "}
              {String(item.page)}:{String(item.firstLine)}–
              {String(item.lastLine)}
            </Button>
          ))}
        </RecordGroup>
        <FormActions label={t("claims")}>
          <Button
            disabled={
              mutable ||
              !statement.trim() ||
              !reason.trim() ||
              !citations.length
            }
            onClick={() => {
              void settle(
                operations.command(accept, {
                  ...scope,
                  id: editing?.id ?? crypto.randomUUID(),
                  expectedRevision: editing?.revision ?? 0,
                  statement: statement.trim(),
                  reason: reason.trim(),
                  citations,
                }),
                (saved) => {
                  setOpened({
                    ...scope,
                    id: saved.id,
                    revision: saved.revision,
                  });
                  setEditing(undefined);
                  setStatement("");
                  setReason("");
                  setCitations([]);
                },
              );
            }}
          >
            {t("accept")}
          </Button>
        </FormActions>
      </PanelSection>
    </>
  );
}

function MoveMaterial({
  context,
  project,
  disabled,
  onMove,
  t,
}: {
  context: ComponentContext;
  project: string;
  disabled: boolean;
  onMove(target: string): void;
  t: Messages;
}) {
  const [projects, setProjects] = useState<Project[]>([]);
  const [selection, setSelection] = useState<string>();
  const [error, setError] = useState<string>();
  useEffect(() => {
    const reader = new AbortController();
    void context
      .track(() =>
        context.runtime.projects
          .list(undefined, AbortSignal.any([context.signal, reader.signal]))
          .autoPagingToArray(),
      )
      .then(
        (values) => {
          if (!reader.signal.aborted)
            setProjects(values.filter((value) => value.id !== project));
        },
        (failure) => {
          if (!reader.signal.aborted) setError(failureText(failure));
        },
      );
    return () => reader.abort();
  }, [context.runtime, context.signal, context.track, project]);
  if (error) return <p role="alert">{error}</p>;
  if (projects.length === 0) return null;
  return (
    <FormActions label={t("move")}>
      <Select
        label={t("targetProject")}
        value={selection ?? null}
        onValueChange={setSelection}
        disabled={disabled}
        options={projects.map((value) => ({
          value: value.id,
          label: value.name,
        }))}
      />
      <Button
        disabled={disabled || !selection}
        onClick={() => {
          if (selection) onMove(selection);
        }}
      >
        {t("move")}
      </Button>
    </FormActions>
  );
}

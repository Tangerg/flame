import { useEffect, useMemo, useState } from "react";
import {
  bindOperations,
  failureText,
  mutationSettlementIsUnknown,
  useTranslator,
  watchRuntimeTopics,
  type PluginComponentProps,
} from "@flame/component-sdk";
import { asRunId, type MutationPromise } from "@flame/runtime-client";
import type { RunRef } from "@flame/runtime-client/wire";
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
const get = "flame.filewatch/get.v1";
const configure = "flame.filewatch/configure.v1";
const status = "flame.filewatch/status.v1";
const events = "flame.filewatch/events.v1";
const event = "flame.filewatch/event.v1";
type Snapshot = MethodOutputs[typeof get];
type Draft = NonNullable<MethodInputs[typeof configure]["trigger"]>;
type Observation = MethodOutputs[typeof event];
type Read = {
  settings: Snapshot;
  status: MethodOutputs[typeof status];
  events: MethodOutputs[typeof events];
};

export function FileWatch({ context, locale }: PluginComponentProps) {
  const t = useTranslator(catalogs, locale);
  const ops = useMemo(() => {
    if (!context.binding)
      throw new Error("File watch requires its own contribution");
    return bindOperations<MethodInputs, MethodOutputs>(
      context,
      CAPABILITY_JSON,
      {
        scope: "installation",
        reference: context.binding.installationId,
      },
    );
  }, [context]);
  const [read, setRead] = useState<Read>();
  const [problem, setProblem] = useState<string>();
  const [mutationError, setMutationError] = useState<string>();
  const [refresh, setRefresh] = useState(0);
  const [selected, setSelected] = useState("");
  const [details, setDetails] = useState<{
    id: string;
    observation?: Observation;
    run?: RunRef;
    error?: string;
  }>();
  const [busy, setBusy] = useState(false);
  const [recover, setRecover] = useState<() => void>();
  const [editor, setEditor] = useState<{
    revision: Snapshot["revision"];
    draft: Draft;
    paths: string;
  }>();
  const latest = read?.events[0];
  const inspected = selected || latest?.id;
  const settings = read?.settings;
  const current = settings?.trigger;

  useEffect(() => {
    const subscription = new AbortController();
    let reader: AbortController | undefined;
    const load = () => {
      reader?.abort();
      const reading = (reader = new AbortController());
      void Promise.all([
        ops.query(get, {}, reading.signal),
        ops.query(status, {}, reading.signal),
        ops.query(events, {}, reading.signal),
      ]).then(
        ([settings, status, events]) => {
          if (!reading.signal.aborted) {
            setRead({ settings, status, events });
            setProblem(undefined);
          }
        },
        (error) => {
          if (!reading.signal.aborted) setProblem(failureText(error));
        },
      );
    };
    load();
    void context
      .track(() =>
        watchRuntimeTopics(
          context.runtime,
          { topics: ["plugins.dataChanged", "runs.changed"] },
          AbortSignal.any([subscription.signal, context.signal]),
          load,
        ),
      )
      .catch((error) => {
        if (!subscription.signal.aborted) setProblem(failureText(error));
      });
    return () => {
      subscription.abort();
      reader?.abort();
    };
  }, [context, ops, refresh]);

  useEffect(() => {
    if (!read || !inspected) return;
    const reader = new AbortController();
    const lookup = async () => {
      const observation = await ops.query(
        event,
        { id: inspected },
        reader.signal,
      );
      if (reader.signal.aborted) return;
      setDetails({ id: inspected, observation });
      const runId = observation.admittedRunId || observation.capture.runId;
      if (!observation.accepted || !runId) return;
      const run = await context.track((signal) =>
        context.runtime.runs.get(
          asRunId(runId),
          AbortSignal.any([reader.signal, signal]),
        ),
      );
      if (!reader.signal.aborted)
        setDetails({ id: inspected, observation, run });
    };
    void lookup().catch((error) => {
      if (!reader.signal.aborted)
        setDetails((current) => ({
          id: inspected,
          observation:
            current?.id === inspected ? current.observation : undefined,
          error: failureText(error),
        }));
    });
    return () => reader.abort();
  }, [context, inspected, ops, read]);

  async function save(operation: MutationPromise<Snapshot>) {
    setBusy(true);
    setMutationError(undefined);
    setRecover(undefined);
    try {
      const saved = await operation;
      setRead((current) => current && { ...current, settings: saved });
      setEditor(undefined);
      setRefresh((value) => value + 1);
    } catch (error) {
      setMutationError(failureText(error));
      if (mutationSettlementIsUnknown(error))
        setRecover(() => () => void save(operation.retry()));
    } finally {
      setBusy(false);
    }
  }

  const session = context.sessionId;
  const workspace = context.workspace?.ref.path;
  const locked = busy || !!recover;
  const begin = () => {
    if (!session || !workspace || !settings) return;
    const same =
      current?.targetSessionId === session && current.workspace === workspace;
    const draft = same
      ? { ...current, projectId: context.project?.id }
      : {
          title: "",
          instructions: "",
          paths: [],
          targetSessionId: session,
          projectId: context.project?.id,
          workspace,
        };
    setEditor({
      revision: settings.revision,
      draft,
      paths: draft.paths.join("\n"),
    });
  };
  const observed = details?.id === inspected ? details : undefined;
  const evidence = observed?.observation?.capture.partitions;
  const time = (value: string) => new Date(value).toLocaleString(locale);

  return (
    <Panel label={t("title")} header={<PanelTitle>{t("title")}</PanelTitle>}>
      <PanelSection title={t("rule")}>
        {!read && !problem && <p role="status">{t("loading")}</p>}
        {problem && <p role="alert">{problem}</p>}
        {mutationError && <p role="alert">{mutationError}</p>}
        {read && (
          <RecordGroup>
            {current && (
              <>
                <RecordBody>{current.title}</RecordBody>
                <Metadata>{current.workspace}</Metadata>
                <Metadata>
                  {current.targetSessionId ?? t("newSession")}
                </Metadata>
              </>
            )}
            <Metadata>
              {t(read.status.phase)} ·{" "}
              {t("revision", { revision: String(settings?.revision) })}
            </Metadata>
            {String(read.status.revision) !== String(settings?.revision) && (
              <Metadata>{t("applying")}</Metadata>
            )}
            {read.status.error && <p role="alert">{read.status.error}</p>}
            {read.status.dispatchError && (
              <p role="status">{read.status.dispatchError}</p>
            )}
          </RecordGroup>
        )}
        <FormActions label={t("rule")}>
          <Button onClick={() => setRefresh((value) => value + 1)}>
            {t("refresh")}
          </Button>
          {recover && <Button onClick={recover}>{t("recover")}</Button>}
          {settings && (
            <>
              <Button
                onClick={begin}
                disabled={!session || !workspace || locked}
              >
                {t("follow")}
              </Button>
              {current && (
                <Button
                  disabled={locked}
                  onClick={() =>
                    void save(
                      ops.command(configure, {
                        expectedRevision: settings.revision,
                      }),
                    )
                  }
                >
                  {t("stop")}
                </Button>
              )}
            </>
          )}
        </FormActions>
        {settings && (!session || !workspace) && (
          <Metadata>{t("chooseSession")}</Metadata>
        )}
        {editor && (
          <form
            onSubmit={(submit) => {
              submit.preventDefault();
              if (locked) return;
              const paths = editor.paths
                .split("\n")
                .map((path) => path.trim())
                .filter(Boolean);
              if (!paths.length) return;
              void save(
                ops.command(configure, {
                  expectedRevision: editor.revision,
                  trigger: { ...editor.draft, paths },
                }),
              );
            }}
          >
            <Metadata>
              {editor.draft.targetSessionId} · {editor.draft.workspace}
            </Metadata>
            <FormField label={t("method")}>
              <TextField
                required
                maxLength={200}
                value={editor.draft.title}
                onChange={(change) =>
                  setEditor({
                    ...editor,
                    draft: { ...editor.draft, title: change.target.value },
                  })
                }
              />
            </FormField>
            <FormField label={t("paths")}>
              <TextArea
                required
                value={editor.paths}
                onChange={(change) =>
                  setEditor({ ...editor, paths: change.target.value })
                }
              />
            </FormField>
            <FormField label={t("instructions")}>
              <TextArea
                required
                maxLength={16384}
                value={editor.draft.instructions}
                onChange={(change) =>
                  setEditor({
                    ...editor,
                    draft: {
                      ...editor.draft,
                      instructions: change.target.value,
                    },
                  })
                }
              />
            </FormField>
            <label>
              <input
                type="checkbox"
                checked={editor.draft.capturePartitions ?? false}
                onChange={(change) =>
                  setEditor({
                    ...editor,
                    draft: {
                      ...editor.draft,
                      capturePartitions: change.target.checked,
                    },
                  })
                }
              />{" "}
              {t("capture")}
            </label>
            <FormActions label={t("rule")}>
              <Button type="submit" disabled={locked}>
                {t("enable")}
              </Button>
              <Button onClick={() => setEditor(undefined)} disabled={locked}>
                {t("dismiss")}
              </Button>
            </FormActions>
          </form>
        )}
      </PanelSection>
      <PanelSection title={t("observations")}>
        {read && read.events.length === 0 && <Metadata>{t("none")}</Metadata>}
        {latest && (
          <FormField label={t("observation")}>
            <Select
              label={t("observation")}
              value={selected || "latest"}
              onValueChange={(value) =>
                setSelected(value === "latest" ? "" : value)
              }
              options={[
                { value: "latest", label: t("latest") },
                ...(read?.events ?? []).map((value) => ({
                  value: value.id,
                  label: `${time(value.observedAt)} · ${t(value.kind)}`,
                })),
              ]}
            />
          </FormField>
        )}
        {observed?.error && <p role="alert">{observed.error}</p>}
        {observed?.observation && (
          <RecordGroup>
            <Metadata>
              {time(observed.observation.capture.observedAt)} ·{" "}
              {t("revision", {
                revision: String(observed.observation.capture.revision),
              })}
            </Metadata>
            <Metadata>
              {t(
                observed.run?.outcome?.type ??
                  observed.run?.status ??
                  (observed.observation.accepted ? "admitted" : "pending"),
              )}
            </Metadata>
            {observed.observation.capture.kind === "resync" && (
              <Metadata>{t("coverageGap")}</Metadata>
            )}
            {evidence && (
              <>
                <Metadata>
                  {t("coverage", {
                    read: evidence.filter((part) => part.state === "read")
                      .length,
                    total: evidence.length,
                  })}
                </Metadata>
                {evidence.map((part) => (
                  <details key={part.path}>
                    <summary>
                      {part.path} · {t(part.state)}
                    </summary>
                    {part.sha256 && <Metadata>SHA-256 {part.sha256}</Metadata>}
                    {part.text !== undefined && (
                      <RecordBody>{part.text}</RecordBody>
                    )}
                  </details>
                ))}
              </>
            )}
            {context.openSession && (
              <Button
                onClick={() => {
                  void context.openSession?.(observed.observation!.capture.sessionId)
                    .catch((error) => {
                      if (!context.signal.aborted) setMutationError(failureText(error));
                    });
                }}
              >
                {t("openSession")}
              </Button>
            )}
          </RecordGroup>
        )}
      </PanelSection>
    </Panel>
  );
}

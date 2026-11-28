import { useEffect, useMemo, useState, useSyncExternalStore } from "react";
import {
  bindOperations,
  failureText,
  watchRuntimeTopics,
  type ComponentContext,
  type PluginComponentProps,
} from "@flame/component-sdk";
import type { TaskWorktree } from "@flame/runtime-client/wire";
import {
  Button,
  FormActions,
  FormField,
  Metadata,
  Panel,
  PanelSection,
  PanelTitle,
  Select,
  TextArea,
  TextField,
} from "@flame/ui";
import {
  CAPABILITY_JSON,
  type MethodInputs,
  type MethodOutputs,
} from "../package/io.github.tangerg.flame/contracts/methods.generated";

const board = "flame.methods/board.v1";
const read = "flame.methods/execution.v1";
const save = "flame.methods/save.v1";
const start = "flame.methods/start.v1";
const approve = "flame.methods/approve.v1";

type Board = MethodOutputs[typeof board];
type Execution = MethodOutputs[typeof read];
type Recipe = Omit<MethodInputs[typeof save], "projectId">;
const blank: Recipe = {
  title: "",
  change: "",
  reviewA: "",
  reviewB: "",
  provider: "",
  model: "",
};
const en = {
  title: "Methods",
  project: "Open Methods from a project.",
  edit: "Edit method",
  save: "Save new version",
  name: "Method name",
  change: "Workspace change",
  reviewA: "Review A",
  reviewB: "Review B",
  provider: "Provider",
  model: "Model",
  worktree: "Managed worktree",
  start: "Run this version",
  executions: "Executions",
  none: "No method executions",
  changes: "Captured changes",
  approve: "Approve report",
  reject: "Reject report",
  rejected: "Rejected",
  run: "Open change conversation",
  review: "Open review conversation",
  recover: "Recover original action",
  queued: "Queued",
  changing: "Making changes",
  reviewing: "Reviewing",
  awaiting_approval: "Awaiting approval",
  approved: "Approved",
  blocked: "Blocked",
  running: "Running",
  waiting: "Waiting for a response",
  failed: "Failed",
  passed: "Passed",
};
const zh: typeof en = {
  title: "方法",
  project: "请从项目中打开方法。",
  edit: "编辑方法",
  save: "保存新版本",
  name: "方法名称",
  change: "工作树改动",
  reviewA: "审核 A",
  reviewB: "审核 B",
  provider: "提供方",
  model: "模型",
  worktree: "受管工作树",
  start: "运行此版本",
  executions: "执行记录",
  none: "暂无方法执行",
  changes: "捕获的改动",
  approve: "批准报告",
  reject: "驳回报告",
  rejected: "已驳回",
  run: "打开改动会话",
  review: "打开审核会话",
  recover: "恢复原操作",
  queued: "已排队",
  changing: "改动中",
  reviewing: "审核中",
  awaiting_approval: "待批准",
  approved: "已批准",
  blocked: "已阻止",
  running: "运行中",
  waiting: "待答复",
  failed: "失败",
  passed: "通过",
};

export function ProjectMethods({ context, locale }: PluginComponentProps) {
  const labels = locale.startsWith("zh") ? zh : en;
  const [target, setTarget] = useState("");
  useEffect(() => context.views?.subscribeTarget(setTarget), [context.views]);
  const [project, execution] = target.split("/");
  const projectID = project || context.project?.id;
  return (
    <Panel
      label={labels.title}
      header={<PanelTitle>{labels.title}</PanelTitle>}
    >
      {projectID ? (
        <MethodBoard
          key={projectID}
          project={projectID}
          target={execution}
          context={context}
          t={labels}
        />
      ) : (
        <p role="status">{labels.project}</p>
      )}
    </Panel>
  );
}

function MethodBoard({
  context,
  project,
  target,
  t,
}: {
  context: ComponentContext;
  project: string;
  target?: string;
  t: typeof en;
}) {
  const ops = useMemo(
    () =>
      bindOperations<MethodInputs, MethodOutputs>(context, CAPABILITY_JSON, {
        scope: "installation",
        reference: context.binding!.installationId,
      }),
    [context],
  );
  const [state, setState] = useState<Board>();
  const [trees, setTrees] = useState<readonly TaskWorktree[]>([]);
  const [worktree, setWorktree] = useState("");
  const [selected, select] = useState<string>();
  const id = selected || target;
  const [detail, setDetail] = useState<Execution>();
  const [draft, edit] = useState<Recipe>();
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [revision, refresh] = useState(0);
  useSyncExternalStore(ops.subscribeCommands, ops.commandsVersion);
  const originals = context.signal.aborted
    ? []
    : ops.commands(save, start, approve);
  const locked = busy || originals.some((entry) => entry.state === "active");

  useEffect(() => {
    const lifetime = new AbortController();
    const signal = AbortSignal.any([context.signal, lifetime.signal]);
    let pending: AbortController | undefined;
    const load = () => {
      pending?.abort();
      pending = new AbortController();
      const request = AbortSignal.any([pending.signal, signal]);
      void Promise.all([
        ops.query(board, { projectId: project }, request),
        context.runtime.workspaces.tasks.list(request),
        id ? ops.query(read, { projectId: project, id }, request) : undefined,
      ]).then(
        ([view, taskTrees, execution]) => {
          if (request.aborted) return;
          setState(view);
          setTrees(taskTrees.data.filter((tree) => tree.available));
          setDetail(execution);
        },
        (cause) => {
          if (request.aborted) return;
          setState(undefined);
          setDetail(undefined);
          setError(failureText(cause));
        },
      );
    };
    void context
      .track(() =>
        watchRuntimeTopics(
          context.runtime,
          { topics: ["plugins.dataChanged"] },
          signal,
          load,
        ),
      )
      .catch(() => {
        if (!signal.aborted) load();
      });
    return () => lifetime.abort();
  }, [ops, context, project, id, revision]);

  async function perform(action: () => Promise<unknown>) {
    setBusy(true);
    setError(undefined);
    try {
      await action();
    } catch (cause) {
      if (!context.signal.aborted) setError(failureText(cause));
    } finally {
      if (!context.signal.aborted) {
        setBusy(false);
        refresh((value) => value + 1);
      }
    }
  }

  const version = state?.latest;
  const execution = detail?.id === id ? detail : undefined;
  return (
    <>
      <PanelSection title={version?.title ?? t.title}>
        {version && (
          <Metadata>
            <code>{version.id}</code>
          </Metadata>
        )}
        {draft ? (
          <form
            onSubmit={(event) => {
              event.preventDefault();
              void perform(async () => {
                await ops.command(save, { ...draft, projectId: project });
                edit(undefined);
              });
            }}
          >
            {(["title", "provider", "model"] as const).map((field) => (
              <FormField
                key={field}
                label={field === "title" ? t.name : t[field]}
              >
                <TextField
                  required
                  value={draft[field]}
                  onChange={(event) =>
                    edit({ ...draft, [field]: event.target.value })
                  }
                />
              </FormField>
            ))}
            {(["change", "reviewA", "reviewB"] as const).map((field) => (
              <FormField key={field} label={t[field]}>
                <TextArea
                  required
                  value={draft[field]}
                  onChange={(event) =>
                    edit({ ...draft, [field]: event.target.value })
                  }
                />
              </FormField>
            ))}
            <Button type="submit" disabled={locked}>
              {t.save}
            </Button>
          </form>
        ) : (
          <Button
            onClick={() =>
              edit(
                version
                  ? {
                      title: version.title,
                      change: version.change,
                      reviewA: version.reviewA,
                      reviewB: version.reviewB,
                      provider: version.provider,
                      model: version.model,
                    }
                  : blank,
              )
            }
            disabled={locked}
          >
            {t.edit}
          </Button>
        )}
        {version && (
          <>
            <FormField label={t.worktree}>
              <Select
                label={t.worktree}
                value={worktree || null}
                options={trees.map(({ id, workspace }) => ({
                  value: id,
                  label: workspace.path,
                }))}
                onValueChange={setWorktree}
                disabled={locked || trees.length === 0}
              />
            </FormField>
            <Button
              disabled={locked || !trees.some((tree) => tree.id === worktree)}
              onClick={() =>
                void perform(async () => {
                  const started = await ops.command(start, {
                    projectId: project,
                    versionId: version.id,
                    worktreeId: worktree,
                  });
                  select(started.id);
                })
              }
            >
              {t.start}
            </Button>
          </>
        )}
        {error && <p role="alert">{error}</p>}
        {originals.map(
          (entry) =>
            entry.state === "recoverable" && (
              <Button
                key={entry.idempotencyKey}
                size="sm"
                disabled={locked}
                onClick={() => void perform(() => entry.recover())}
              >
                {t.recover}
              </Button>
            ),
        )}
      </PanelSection>
      <PanelSection title={t.executions}>
        {state?.executions.length === 0 && <p>{t.none}</p>}
        <ul>
          {state?.executions.map((item) => (
            <li key={item.id}>
              <Button
                size="sm"
                variant="ghost"
                aria-pressed={id === item.id}
                onClick={() => select(item.id)}
              >
                {item.version.title} ·{" "}
                {t[item.state as keyof typeof en] ?? item.state}
              </Button>
            </li>
          ))}
        </ul>
      </PanelSection>
      {execution && (
        <PanelSection title={execution.version.title}>
          <Metadata>
            {t[execution.state as keyof typeof en] ?? execution.state} ·{" "}
            {execution.version.provider}/{execution.version.model}
          </Metadata>
          <Metadata>
            <code>{execution.version.id}</code>
          </Metadata>
          <Metadata>
            {execution.workspace} · <code>{execution.base}</code>
          </Metadata>
          {execution.reason && <p role="status">{execution.reason}</p>}
          <FormActions label={t.executions}>
            {execution.change && (
              <Button
                onClick={() =>
                  void perform(
                    () =>
                      context.openSession?.(execution.change!.sessionId) ??
                      Promise.resolve(),
                  )
                }
              >
                {t.run}
              </Button>
            )}
            {execution.review && (
              <Button
                onClick={() =>
                  void perform(
                    () =>
                      context.openSession?.(execution.review!.sessionId) ??
                      Promise.resolve(),
                  )
                }
              >
                {t.review}
              </Button>
            )}
          </FormActions>
          {execution.checks.map((check) => (
            <div key={check.runId}>
              <Metadata>
                {check.slot === "A" ? t.reviewA : t.reviewB} ·{" "}
                {t[check.status as keyof typeof en] ?? check.status}
              </Metadata>
              {check.result && (
                <p className="whitespace-pre-wrap break-words">
                  {check.result}
                </p>
              )}
            </div>
          ))}
          {execution.handoff && (
            <details>
              <summary>{t.changes}</summary>
              <pre className="whitespace-pre-wrap break-words">
                {execution.patch}
              </pre>
            </details>
          )}
          {execution.state === "awaiting_approval" && execution.digest && (
            <FormActions label={t.awaiting_approval}>
              <Button
                variant="primary"
                disabled={locked}
                onClick={() =>
                  void perform(() =>
                    ops.command(approve, {
                      projectId: project,
                      id: execution.id,
                      digest: execution.digest!,
                    }),
                  )
                }
              >
                {t.approve}
              </Button>
              <Button
                variant="ghost"
                disabled={locked}
                onClick={() =>
                  void perform(() =>
                    ops.command(approve, {
                      projectId: project,
                      id: execution.id,
                      digest: execution.digest!,
                      decision: "reject",
                    }),
                  )
                }
              >
                {t.reject}
              </Button>
            </FormActions>
          )}
          {execution.decision && (
            <Metadata>
              {execution.decision.disposition === "reject"
                ? t.rejected
                : t.approved}{" "}
              · {execution.decision.at}
            </Metadata>
          )}
        </PanelSection>
      )}
    </>
  );
}

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
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
  TextArea,
  TextField,
} from "@flame/ui";
import {
  type TaskWorktree,
  type WorktreeHandoff,
  type MutationPromise,
  mutationSettlementIsUnknown,
} from "@flame/runtime-client";
import {
  useActiveSession,
  useAgentSessions,
  useCreateSession,
} from "@/features/agent/public/session";
import { useAgent } from "@/features/agent/public/agent";
import { useRuntime } from "@/features/connection/public/runtime";
import { useRuntimeCommandsAvailable } from "@/features/connection/public/serviceStatus";
import { selectWorkspaceChat } from "@/features/workspace/public/navigation";
import { useWorkspace } from "@/features/workspace/public/workspace";
import { useT } from "@/lib/i18n/index";
import { rpcErrorText } from "@/lib/rpcErrors";

export function TaskWorktrees() {
  const workspace = useWorkspace();
  const runtime = useRuntime();
  const commandAvailable = useRuntimeCommandsAvailable();
  const active = useActiveSession();
  const agent = useAgent();
  const startSession = useCreateSession();
  const { data: sessions } = useAgentSessions();
  const t = useT();
  const [source, setSource] = useState<string>();
  const [base, setBase] = useState("HEAD");
  const [setup, setSetup] = useState("");
  const [preview, setPreview] = useState<WorktreeHandoff>();
  const [message, setMessage] = useState<string>();
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [recover, setRecover] = useState<() => void>();
  const location = source ?? active?.workspace.path ?? "";
  const tasks = useQuery({
    queryKey: ["taskWorktrees", runtime.endpoint.currentRuntimeEndpoint()],
    queryFn: ({ signal }) => workspace.taskWorktrees().list(signal),
    enabled: commandAvailable,
  });

  const perform = async (run: () => Promise<unknown>) => {
    setBusy(true);
    setError(undefined);
    setMessage(undefined);
    try {
      await run();
    } catch (cause) {
      setError(rpcErrorText(cause) ?? t("workspace.tasks.failed"));
    } finally {
      setBusy(false);
    }
  };
  const settle = <T,>(request: MutationPromise<T>, accept: (value: T) => void) => {
    void perform(async () => {
      setRecover(undefined);
      try {
        accept(await request);
        await tasks.refetch();
      } catch (cause) {
        if (mutationSettlementIsUnknown(cause)) {
          setRecover(() => () => settle(request.retry(), accept));
        }
        throw cause;
      }
    });
  };
  const open = async (task: TaskWorktree) => {
    const bound = sessions?.find((session) => session.workspace.path === task.workspace.path);
    if (bound) agent.selectAgentSession(bound.id);
    else if (!(await startSession({ cwd: task.workspace.path, projectId: active?.projectId })))
      return;
    selectWorkspaceChat(workspace);
  };
  const disabled = busy || !commandAvailable || !!recover;

  return (
    <Panel
      label={t("workspace.tasks.title")}
      header={<PanelTitle>{t("workspace.tasks.title")}</PanelTitle>}
    >
      {error && <p role="alert">{error}</p>}
      {tasks.error && (
        <p role="alert">{rpcErrorText(tasks.error) ?? t("workspace.tasks.failed")}</p>
      )}
      {message && <p role="status">{message}</p>}
      {recover && (
        <Button disabled={busy || !commandAvailable} onClick={recover}>
          {t("workspace.tasks.recover")}
        </Button>
      )}
      <PanelSection title={t("workspace.tasks.create")}>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            if (disabled || !location.trim() || !base.trim()) return;
            settle(
              workspace
                .taskWorktrees()
                .create({
                  source: { path: location.trim() },
                  base: base.trim(),
                  setup: setup.trim() ? setup : undefined,
                }),
              () => setSetup(""),
            );
          }}
        >
          <FormField label={t("session.directory.path")}>
            <TextField value={location} onChange={(event) => setSource(event.target.value)} />
          </FormField>
          <FormField label={t("workspace.tasks.base")}>
            <TextField value={base} onChange={(event) => setBase(event.target.value)} />
          </FormField>
          <FormField label={t("workspace.tasks.setup")}>
            <TextArea value={setup} onChange={(event) => setSetup(event.target.value)} />
          </FormField>
          <FormActions label={t("workspace.tasks.create")}>
            <Button
              type="submit"
              variant="primary"
              disabled={disabled || !location.trim() || !base.trim()}
            >
              {t("workspace.tasks.create")}
            </Button>
          </FormActions>
        </form>
      </PanelSection>
      <PanelSection title={t("workspace.tasks.title")}>
        {tasks.data?.data.map((task) => (
          <RecordGroup key={task.id}>
            <RecordBody>
              <code className="break-all">{task.workspace.path}</code>
            </RecordBody>
            <Metadata>
              <code>{task.base}</code>
            </Metadata>
            {task.setup && (
              <details>
                <summary>
                  {t("workspace.tasks.setup")}
                  {task.setup.exitCode === undefined ? "" : ` · ${task.setup.exitCode}`}
                </summary>
                <pre className="whitespace-pre-wrap break-all">
                  {task.setup.command}
                  {"\n"}
                  {task.setup.output}
                </pre>
              </details>
            )}
            <FormActions label={t("workspace.tasks.title")}>
              <Button disabled={disabled || !task.available} onClick={() => void open(task)}>
                {t("workspace.tasks.open")}
              </Button>
              <Button
                disabled={disabled || !task.available}
                onClick={() =>
                  void perform(async () => {
                    setPreview(undefined);
                    setPreview(await workspace.taskWorktrees().preview(task.id));
                  })
                }
              >
                {t("workspace.tasks.preview")}
              </Button>
              <Button
                disabled={disabled}
                onClick={() =>
                  settle(workspace.taskWorktrees().release(task.id), () => setPreview(undefined))
                }
              >
                {t("workspace.tasks.release")}
              </Button>
            </FormActions>
          </RecordGroup>
        ))}
      </PanelSection>
      {preview && (
        <PanelSection title={t("workspace.tasks.preview")}>
          <Metadata>
            <code>
              {preview.branch} · {preview.head}
            </code>
          </Metadata>
          <RecordBody>
            <code className="break-all">{preview.worktree.source.path}</code>
          </RecordBody>
          <pre className="whitespace-pre-wrap break-words">{preview.patch}</pre>
          <FormActions label={t("workspace.tasks.preview")}>
            <Button
              variant="primary"
              disabled={disabled || !preview.patch}
              onClick={() => {
                settle(
                  workspace
                    .taskWorktrees()
                    .apply({ id: preview.worktree.id, digest: preview.digest }),
                  () => {
                    setPreview(undefined);
                    setMessage(t("workspace.tasks.applied"));
                  },
                );
              }}
            >
              {t("workspace.tasks.apply")}
            </Button>
            <Button disabled={busy} onClick={() => setPreview(undefined)}>
              {t("common.cancel")}
            </Button>
          </FormActions>
        </PanelSection>
      )}
    </Panel>
  );
}

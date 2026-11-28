import { useState, useSyncExternalStore } from "react";
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
import { type TaskWorktree, type WorktreeHandoff } from "@flame/runtime-client";
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
import { type TaskWorktreeMutation } from "@/features/workspace/application/taskWorktreeCommands";
import { taskWorktreesQueryKey } from "@/features/workspace/application/workspaceQueries";
import { useT } from "@/lib/i18n/index";
import { rpcErrorText } from "@/lib/rpcErrors";

const DEFAULT_BASE_REF = "HEAD";

type PreviewState =
  | { phase: "idle" | "loading" }
  | { phase: "ready"; handoff: WorktreeHandoff }
  | { phase: "failed"; error: unknown };

export function TaskWorktrees() {
  const workspace = useWorkspace();
  const runtime = useRuntime();
  const commands = workspace.taskWorktreeCommands;
  const command = useSyncExternalStore(commands.subscribe, commands.snapshot);
  const commandAvailable = useRuntimeCommandsAvailable();
  const active = useActiveSession();
  const agent = useAgent();
  const startSession = useCreateSession();
  const { data: sessions } = useAgentSessions();
  const t = useT();
  const [source, setSource] = useState<string>();
  const [base, setBase] = useState(DEFAULT_BASE_REF);
  const [setup, setSetup] = useState("");
  const [previewState, setPreview] = useState<PreviewState>({ phase: "idle" });
  const preview = previewState.phase === "ready" ? previewState.handoff : undefined;
  const error =
    command.phase === "failed" || command.phase === "unconfirmed"
      ? command.error
      : previewState.phase === "failed"
        ? previewState.error
        : undefined;
  const location = source ?? active?.workspace.path ?? "";
  const tasks = useQuery({
    queryKey: taskWorktreesQueryKey(runtime.endpoint.currentRuntimeEndpoint()),
    queryFn: ({ signal }) => workspace.taskWorktrees().list(signal),
    enabled: commandAvailable,
  });

  const showPreview = async (id: string) => {
    setPreview({ phase: "loading" });
    try {
      setPreview({ phase: "ready", handoff: await workspace.taskWorktrees().preview(id) });
    } catch (cause) {
      setPreview({ phase: "failed", error: cause });
    }
  };
  const settle = async (
    operation: TaskWorktreeMutation,
    acknowledged: Promise<boolean>,
    submittedSetup?: string,
  ) => {
    if (!(await acknowledged)) return;
    if (operation !== "create") setPreview({ phase: "idle" });
    else if (submittedSetup !== undefined)
      setSetup((current) => (current === submittedSetup ? "" : current));
  };
  const open = async (task: TaskWorktree) => {
    const bound = sessions?.find((session) => session.workspace.path === task.workspace.path);
    if (bound) agent.selectAgentSession(bound.id);
    else if (!(await startSession({ cwd: task.workspace.path, projectId: active?.projectId })))
      return;
    selectWorkspaceChat(workspace);
  };
  const busy = command.phase === "sending" || previewState.phase === "loading";
  const disabled = busy || !commandAvailable || command.phase === "unconfirmed";

  return (
    <Panel
      label={t("workspace.tasks.title")}
      header={<PanelTitle>{t("workspace.tasks.title")}</PanelTitle>}
    >
      {error !== undefined && (
        <p role="alert">{rpcErrorText(error) ?? t("workspace.tasks.failed")}</p>
      )}
      {tasks.error && (
        <p role="alert">{rpcErrorText(tasks.error) ?? t("workspace.tasks.failed")}</p>
      )}
      {command.phase === "committed" && command.operation === "apply" && (
        <p role="status">{t("workspace.tasks.applied")}</p>
      )}
      {command.phase === "unconfirmed" && (
        <Button
          disabled={busy || !commandAvailable}
          onClick={() => void settle(command.operation, commands.recover())}
        >
          {t("workspace.tasks.recover")}
        </Button>
      )}
      <PanelSection title={t("workspace.tasks.create")}>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            if (disabled || !location.trim() || !base.trim()) return;
            void settle(
              "create",
              commands.create({
                source: { path: location.trim() },
                base: base.trim(),
                setup: setup.trim() ? setup : undefined,
              }),
              setup,
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
                onClick={() => void showPreview(task.id)}
              >
                {t("workspace.tasks.preview")}
              </Button>
              <Button
                disabled={disabled}
                onClick={() => void settle("release", commands.release(task.id))}
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
              onClick={() =>
                void settle(
                  "apply",
                  commands.apply({ id: preview.worktree.id, digest: preview.digest }),
                )
              }
            >
              {t("workspace.tasks.apply")}
            </Button>
            <Button disabled={busy} onClick={() => setPreview({ phase: "idle" })}>
              {t("common.cancel")}
            </Button>
          </FormActions>
        </PanelSection>
      )}
    </Panel>
  );
}

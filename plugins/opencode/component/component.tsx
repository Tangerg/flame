import { useEffect, useMemo, useState } from "react";
import {
  bindOperations,
  failureText,
  mutationSettlementIsUnknown,
  type ComponentContext,
} from "@flame/component-sdk";
import { type MutationPromise } from "@flame/runtime-client";
import {
  Button,
  FormActions,
  FormField,
  Metadata,
  Panel,
  PanelSection,
  PanelTitle,
  TextArea,
  TextField,
} from "@flame/ui";
import {
  CAPABILITY_JSON,
  type MethodInputs,
  type MethodOutputs,
} from "../package/io.github.tangerg.flame/contracts/methods.generated";

const list = "flame.opencode/list.v1";
const open = "flame.opencode/open.v1";
const get = "flame.opencode/get.v1";
const prompt = "flame.opencode/prompt.v1";
const cancel = "flame.opencode/cancel.v1";
const permission = "flame.opencode/permission.v1";
const fork = "flame.opencode/fork.v1";
const view = "opencode-sessions";

type Session = MethodOutputs[typeof get];

const english = {
  title: "OpenCode sessions",
  directory: "Workspace directory",
  list: "Find native sessions",
  next: "Next results",
  nativeId: "Native session ID",
  open: "Open native session",
  connecting: "Reading the original session…",
  reconnect: "Reconnect",
  fork: "Fork session",
  cancel: "Request cancellation",
  cancelPending: "Cancellation awaiting the original turn",
  offline: "Connection unavailable",
  unconfirmed: "Turn outcome is unconfirmed",
  approve: "Tool permission",
  progress: "Session activity",
  prompt: "Message",
  send: "Send",
  continue: "Continue in a new turn",
  retry: "Check original command",
  diff: "Read workspace diff",
  clean: "The workspace has no Git changes.",
  error: "OpenCode request failed.",
};
const chinese: typeof english = {
  title: "OpenCode 会话",
  directory: "工作目录",
  list: "查找原生会话",
  next: "下一页",
  nativeId: "原生会话 ID",
  open: "打开原生会话",
  connecting: "正在读取原会话…",
  reconnect: "重新连接",
  fork: "分叉会话",
  cancel: "请求取消",
  cancelPending: "正在等待原轮次确认取消",
  offline: "连接不可用",
  unconfirmed: "轮次结果待确认",
  approve: "工具权限",
  progress: "会话进度",
  prompt: "消息",
  send: "发送",
  continue: "开始新一轮",
  retry: "查询原命令",
  diff: "查看工作目录差异",
  clean: "工作目录没有 Git 改动。",
  error: "OpenCode 请求失败。",
};

export function OpenCodeSessions({ context }: { context: ComponentContext }) {
  const t = context.locale.startsWith("zh") ? chinese : english;
  const ops = useMemo(() => {
    if (!context.binding)
      throw new Error("OpenCode requires its own installation");
    return bindOperations<MethodInputs, MethodOutputs>(
      context,
      CAPABILITY_JSON,
      {
        scope: "installation",
        reference: context.binding.installationId,
      },
    );
  }, [context]);
  const [directory, setDirectory] = useState(context.workspace?.ref.path ?? "");
  const [nativeId, setNativeId] = useState("");
  const [sessions, setSessions] = useState<MethodOutputs[typeof list]>();
  const [id, setID] = useState("");
  const [current, setCurrent] = useState<Session>();
  const [message, setMessage] = useState("");
  const [diff, setDiff] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [readError, setReadError] = useState("");
  const [recover, setRecover] = useState<MutationPromise<Session>>();

  useEffect(
    () =>
      context.views?.subscribeTarget((value) => {
        if (/^[a-f0-9]{64}$/.test(value)) setID(value);
      }),
    [context.views],
  );
  useEffect(() => {
    if (!id) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    const read = async () => {
      try {
        const result = await context.track((signal) =>
          ops.query(get, { id }, AbortSignal.any([signal, controller.signal])),
        );
        if (!controller.signal.aborted) {
          setCurrent(result);
          setReadError("");
        }
      } catch (cause) {
        if (!controller.signal.aborted)
          setReadError(failureText(cause) ?? t.error);
      } finally {
        if (!controller.signal.aborted)
          timer = setTimeout(() => void read(), 1000);
      }
    };
    void read();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [context, id, ops, t.error]);

  const active = current?.id === id ? current : undefined;
  const turn = active?.turn;
  const locked = busy || !!recover;
  async function find(cursor?: string) {
    setBusy(true);
    setError("");
    try {
      const page = await context.track((signal) =>
        ops.query(list, { cwd: directory.trim(), cursor }, signal),
      );
      setSessions(page);
    } catch (cause) {
      setError(failureText(cause) ?? t.error);
    } finally {
      setBusy(false);
    }
  }
  async function settle(
    command: MutationPromise<Session>,
    origin?: HTMLElement,
  ) {
    setBusy(true);
    setError("");
    try {
      const accepted = await command;
      setRecover(undefined);
      if (accepted.id !== id) {
        setCurrent(accepted);
        setID(accepted.id);
        setDiff(undefined);
        context.views?.open(view, accepted.id, origin);
      }
      setMessage("");
    } catch (cause) {
      setError(failureText(cause) ?? t.error);
      setRecover(mutationSettlementIsUnknown(cause) ? command : undefined);
    } finally {
      setBusy(false);
    }
  }
  function openSession(cwd: string, nativeId: string, origin?: HTMLElement) {
    void settle(
      context.track(() => ops.command(open, { cwd, nativeId })),
      origin,
    );
  }
  async function showDiff(session: Session) {
    setError("");
    try {
      const result = await context.track((signal) =>
        context.runtime
          .workspace({ path: session.cwd })
          .diff.get({ format: "raw" }, signal),
      );
      setDiff(result.type === "raw" ? result.patch : "");
    } catch (cause) {
      setError(failureText(cause) ?? t.error);
    }
  }

  return (
    <Panel label={t.title} header={<PanelTitle>{t.title}</PanelTitle>}>
      <PanelSection title={t.open}>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            if (nativeId.trim()) openSession(directory.trim(), nativeId.trim());
            else void find();
          }}
        >
          <FormField label={t.directory}>
            <TextField
              value={directory}
              onChange={(event) => {
                setDirectory(event.target.value);
                setSessions(undefined);
              }}
              required
              disabled={busy}
            />
          </FormField>
          <FormField label={t.nativeId}>
            <TextField
              value={nativeId}
              onChange={(event) => setNativeId(event.target.value)}
              disabled={busy}
            />
          </FormField>
          <FormActions label={t.open}>
            <Button
              type="button"
              size="sm"
              disabled={busy || !directory.trim()}
              onClick={() => void find()}
            >
              {t.list}
            </Button>
            <Button
              type="submit"
              size="sm"
              disabled={locked || !directory.trim() || !nativeId.trim()}
            >
              {t.open}
            </Button>
          </FormActions>
        </form>
        {sessions && (
          <>
            <Metadata>
              {sessions.agent} · {sessions.machine}
            </Metadata>
            <ul>
              {sessions.rows.map((session) => (
                <li key={session.id}>
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={locked}
                    onClick={(event) =>
                      openSession(session.cwd, session.id, event.currentTarget)
                    }
                  >
                    {session.title || session.id}
                  </Button>
                </li>
              ))}
            </ul>
            {sessions.nextCursor && (
              <Button
                size="sm"
                disabled={busy}
                onClick={() => void find(sessions.nextCursor)}
              >
                {t.next}
              </Button>
            )}
          </>
        )}
        {(error || readError) && <p role="alert">{error || readError}</p>}
        {recover && (
          <Button
            size="sm"
            pending={busy}
            onClick={() => void settle(recover.retry())}
          >
            {t.retry}
          </Button>
        )}
        {id && !active && !readError && <p role="status">{t.connecting}</p>}
      </PanelSection>
      {active && (
        <PanelSection title={t.progress}>
          <Metadata>
            {active.agent} · {active.machine} · {active.cwd}
          </Metadata>
          <Metadata>{active.nativeId}</Metadata>
          <p role="status">
            {!active.connected && t.offline}
            {turn?.status === "unconfirmed" && <span> {t.unconfirmed}</span>}
            {turn?.cancelRequested && turn.status !== "finished" && (
              <span> {t.cancelPending}</span>
            )}
            {turn?.stopReason && <span> {turn.stopReason}</span>}
          </p>
          <FormActions label={t.progress}>
            {!active.connected && (
              <Button
                size="sm"
                disabled={locked}
                onClick={() => openSession(active.cwd, active.nativeId)}
              >
                {t.reconnect}
              </Button>
            )}
            {active.canFork && turn?.status !== "running" && (
              <Button
                size="sm"
                disabled={locked}
                onClick={(event) =>
                  void settle(ops.command(fork, { id }), event.currentTarget)
                }
              >
                {t.fork}
              </Button>
            )}
            {turn && turn.status !== "finished" && (
              <Button
                size="sm"
                disabled={locked || turn.cancelRequested}
                onClick={() =>
                  void settle(ops.command(cancel, { id, turnId: turn.id }))
                }
              >
                {t.cancel}
              </Button>
            )}
            <Button size="sm" onClick={() => void showDiff(active)}>
              {t.diff}
            </Button>
          </FormActions>
          {diff !== undefined && <pre>{diff || t.clean}</pre>}
          {active.events.length > 0 && (
            <ol>
              {active.events.map((event, index) => (
                <li key={index}>
                  <Metadata>
                    {event.kind}
                    {event.status && ` · ${event.status}`}
                  </Metadata>{" "}
                  {event.text || event.toolId}
                </li>
              ))}
            </ol>
          )}
          {active.permissions
            .filter((item) => item.status !== "closed")
            .map((item) => (
              <section key={item.id} aria-label={t.approve}>
                <p>
                  {item.title} · {item.status}
                </p>
                {item.preview && <pre>{item.preview}</pre>}
                <FormActions label={t.approve}>
                  {item.options.map((option) => (
                    <Button
                      key={option.id}
                      size="sm"
                      disabled={
                        locked ||
                        !active.connected ||
                        item.status !== "requested" ||
                        turn?.cancelRequested
                      }
                      onClick={() =>
                        void settle(
                          ops.command(permission, {
                            id,
                            permissionId: item.id,
                            optionId: option.id,
                          }),
                        )
                      }
                    >
                      {option.name}
                    </Button>
                  ))}
                </FormActions>
              </section>
            ))}
          <form
            onSubmit={(event) => {
              event.preventDefault();
              void settle(
                ops.command(prompt, {
                  id,
                  previousTurn: turn?.id,
                  text: message,
                  continue: turn?.status === "unconfirmed",
                }),
              );
            }}
          >
            <FormField label={t.prompt}>
              <TextArea
                value={message}
                onChange={(event) => setMessage(event.target.value)}
              />
            </FormField>
            <FormActions label={t.prompt}>
              <Button
                type="submit"
                disabled={
                  locked ||
                  !message.trim() ||
                  !active.connected ||
                  turn?.status === "running"
                }
              >
                {turn?.status === "unconfirmed" ? t.continue : t.send}
              </Button>
            </FormActions>
          </form>
        </PanelSection>
      )}
    </Panel>
  );
}

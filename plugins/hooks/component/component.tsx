import {
  useCallback,
  useEffect,
  useEffectEvent,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  createTranslator,
  failureText,
  mutationSettlementIsUnknown,
  type ComponentContext,
} from "@flame/component-sdk";
import { type MutationPromise } from "@flame/runtime-client";
import {
  type HookInfo,
  type HooksListResult,
  type WorkspaceRef,
} from "@flame/runtime-client/wire";
import { Refresh } from "@flame/icons/react";
import {
  Badge,
  Button,
  EmptyState,
  IconButton,
  ListRow,
  Metadata,
  RecordBody,
  RecordGroup,
  Switch,
  Toolbar,
} from "@flame/ui";
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { de } from "./locales/de";

const CATALOGS = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };
type Read = {
  loading: boolean;
  accepted?: { result: HooksListResult; workspace: WorkspaceRef };
  error?: string;
};
type Command = { phase: "running" } | { phase: "unknown"; error: string };
const literal = { whiteSpace: "pre-wrap", overflowWrap: "anywhere" } as const;

export function Hooks({ context }: { context: ComponentContext }) {
  const t = useMemo(
    () => createTranslator(CATALOGS, context.locale),
    [context.locale],
  );
  const [read, setRead] = useState<Read>({ loading: true });
  const reading = useRef<AbortController>(undefined);
  const [observationError, setObservationError] = useState<string>();
  const pending = useRef<MutationPromise<void>>(undefined);
  const [command, setCommand] = useState<Command>();
  const [commandError, setCommandError] = useState<string>();
  const refresh = useCallback(async () => {
    if (context.signal.aborted || (context.sessionId && !context.workspace))
      return;
    reading.current?.abort();
    const controller = new AbortController();
    reading.current = controller;
    const signal = AbortSignal.any([controller.signal, context.signal]);
    setRead((previous) => ({ ...previous, loading: true, error: undefined }));
    try {
      const accepted = await context.track(async () => {
        const workspace = await context.runtime.workspaces.open(
          context.workspace?.ref,
          signal,
        );
        return {
          result: await workspace.hooks.list(signal),
          workspace: workspace.ref,
        };
      });
      if (!signal.aborted) setRead({ loading: false, accepted });
    } catch (error) {
      if (!signal.aborted)
        setRead((previous) => ({
          ...previous,
          loading: false,
          error: failureText(error),
        }));
    }
  }, [context]);
  useEffect(() => {
    void refresh();
    return () => reading.current?.abort();
  }, [refresh]);
  const refreshFromEvent = useEffectEvent(() => {
    void refresh();
  });
  const workspacePath = read.accepted?.workspace.path;
  useEffect(() => {
    if (!workspacePath || observationError) return;
    const controller = new AbortController();
    const signal = AbortSignal.any([context.signal, controller.signal]);
    void context.track(async () => {
      try {
        const subscription = await context.runtime.runtimeEvents.subscribe(
          {
            topics: ["hooks.changed", "plugins.changed", "files.changed"],
            watches: [
              { watchId: "hook-settings", workspace: { path: workspacePath } },
            ],
          },
          signal,
        );
        if (signal.aborted) return;
        refreshFromEvent();
        for await (const event of subscription.events) {
          if (signal.aborted) break;
          if (
            event.type === "hooks.changed" ||
            event.type === "plugins.changed" ||
            event.type === "files.changed" ||
            event.type === "resync"
          )
            refreshFromEvent();
        }
      } catch (error) {
        if (!signal.aborted) setObservationError(failureText(error));
      }
    });
    return () => controller.abort();
  }, [context, workspacePath, observationError]);

  async function observe(original: MutationPromise<void>, recover: boolean) {
    setCommand({ phase: "running" });
    try {
      await (recover
        ? context.track(() => original.retry({ signal: context.signal }))
        : original);
      if (context.signal.aborted) return;
      pending.current = undefined;
      setCommand(undefined);
      await refresh();
    } catch (error) {
      if (context.signal.aborted) return;
      if (mutationSettlementIsUnknown(error))
        setCommand({ phase: "unknown", error: failureText(error) });
      else {
        pending.current = undefined;
        setCommand(undefined);
        setCommandError(failureText(error));
      }
    }
  }
  function trust(trusted: boolean) {
    const root = read.accepted?.result.projectRoot;
    if (!root || pending.current || context.signal.aborted) return;
    setCommandError(undefined);
    try {
      const original = context.track(() =>
        context.runtime.hooks.setTrust(root, trusted),
      );
      pending.current = original;
      void observe(original, false);
    } catch (error) {
      setCommandError(failureText(error));
    }
  }
  const snapshot = read.accepted?.result;
  return (
    <>
      <Toolbar label={t("title")}>
        <IconButton
          icon={Refresh}
          aria-label={t("refresh")}
          pending={read.loading}
          onClick={() => {
            setObservationError(undefined);
            void refresh();
          }}
        />
      </Toolbar>
      {read.loading && !snapshot && <p role="status">{t("reading")}</p>}
      {read.error && (
        <div role="alert">
          <RecordBody>{read.error}</RecordBody>
          {snapshot && <Metadata>{t("read.retained")}</Metadata>}
          <Button onClick={() => void refresh()}>{t("retry")}</Button>
        </div>
      )}
      {observationError && (
        <div role="alert">
          <RecordBody>{observationError}</RecordBody>
          <Button onClick={() => setObservationError(undefined)}>
            {t("retry")}
          </Button>
        </div>
      )}
      {commandError && (
        <p role="alert">
          {t("error.trust")} {commandError}
        </p>
      )}
      {command?.phase === "unknown" && (
        <div role="alert">
          <RecordBody>{t("command.uncertain")}</RecordBody>
          <RecordBody>{command.error}</RecordBody>
          <Button
            onClick={() => {
              if (pending.current) void observe(pending.current, true);
            }}
          >
            {t("command.reconcile")}
          </Button>
        </div>
      )}
      {snapshot?.projectRoot &&
        snapshot.hooks.some((value) => value.scope === "project") && (
          <RecordGroup variant="list" aria-busy={command?.phase === "running"}>
            <ListRow style={{ display: "flex", alignItems: "flex-start" }}>
              <div style={{ minWidth: 0, flex: 1 }}>
                <strong>{t("trust")}</strong>
                <RecordBody>{t("trust.sub")}</RecordBody>
                <code style={literal}>{snapshot.projectRoot}</code>
              </div>
              <Switch
                checked={snapshot.projectTrusted}
                disabled={Boolean(command) || read.loading}
                onCheckedChange={trust}
                ariaLabel={t("trust.aria")}
              />
            </ListRow>
          </RecordGroup>
        )}
      {snapshot &&
        (snapshot.hooks.length ? (
          <RecordGroup variant="list">
            {snapshot.hooks.map((value, index) => (
              <HookRow
                key={`${value.source}:${value.event}:${index}`}
                value={value}
                t={t}
              />
            ))}
          </RecordGroup>
        ) : (
          <EmptyState title={t("empty")} sub={t("empty.sub")} />
        ))}
    </>
  );
}

function HookRow({
  value,
  t,
}: {
  value: HookInfo;
  t: ReturnType<typeof createTranslator>;
}) {
  return (
    <ListRow>
      <div
        style={{ display: "grid", minWidth: 0, gap: "var(--layout-label-gap)" }}
      >
        <div
          style={{
            display: "flex",
            flexWrap: "wrap",
            gap: "var(--layout-control-gap)",
            alignItems: "center",
          }}
        >
          <Badge>{value.event}</Badge>
          <Metadata>{t(`scope.${value.scope}`)}</Metadata>
          {!value.active ? (
            <Badge tone="warning" title={t("inactive.hint")}>
              {t("inactive")}
            </Badge>
          ) : value.inject ? (
            <Metadata>{t("kind.inject")}</Metadata>
          ) : null}
        </div>
        {value.matcher && (
          <code title={t("matcher")} style={literal}>
            {value.matcher}
          </code>
        )}
        <code
          style={{ ...literal, fontStyle: value.inject ? "italic" : undefined }}
        >
          {value.command || value.inject}
        </code>
        <Metadata>
          <code style={literal}>{value.source}</code>
        </Metadata>
      </div>
    </ListRow>
  );
}

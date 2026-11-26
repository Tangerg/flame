import {
  useCallback,
  useEffect,
  useEffectEvent,
  useRef,
  useState,
} from "react";
import {
  failureText,
  mutationSettlementIsUnknown,
  type ComponentContext,
  type Translate,
  useTranslator,
  watchRuntimeTopics,
} from "@flame/component-sdk";
import { type MutationPromise } from "@flame/runtime-client";
import {
  type HookInfo,
  type HookScope,
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
  PanelSection,
  RecordBody,
  RecordGroup,
  RecordHeading,
  RecordMeta,
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
// One trust command at a time; an unknown settlement keeps its original
// command until a check through the same idempotency key settles it.
type Command =
  | { phase: "running"; original: MutationPromise<void> }
  | { phase: "unknown"; original: MutationPromise<void>; error: string };
const literal = { whiteSpace: "pre-wrap", overflowWrap: "anywhere" } as const;

export function Hooks({ context }: { context: ComponentContext }) {
  const { runtime, track, signal: componentSignal, sessionId } = context;
  const workspaceRef = context.workspace?.ref;
  const t = useTranslator(CATALOGS, context.locale);
  const [read, setRead] = useState<Read>({ loading: true });
  const reading = useRef<AbortController>(undefined);
  const [observationError, setObservationError] = useState<string>();
  const [command, setCommand] = useState<Command>();
  const [commandError, setCommandError] = useState<string>();
  const refresh = useCallback(async () => {
    if (componentSignal.aborted || (sessionId && !workspaceRef)) return;
    reading.current?.abort();
    const controller = new AbortController();
    reading.current = controller;
    const signal = AbortSignal.any([controller.signal, componentSignal]);
    setRead((previous) => ({ ...previous, loading: true, error: undefined }));
    try {
      const accepted = await track(async () => {
        const workspace = await runtime.workspaces.open(workspaceRef, signal);
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
  }, [runtime, track, componentSignal, workspaceRef, sessionId]);
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
    const signal = AbortSignal.any([componentSignal, controller.signal]);
    void track(async () => {
      try {
        await watchRuntimeTopics(
          runtime,
          {
            topics: ["hooks.changed", "plugins.changed", "files.changed"],
            watches: [
              { watchId: "hook-settings", workspace: { path: workspacePath } },
            ],
          },
          signal,
          refreshFromEvent,
        );
      } catch (error) {
        if (!signal.aborted) setObservationError(failureText(error));
      }
    });
    return () => controller.abort();
  }, [runtime, track, componentSignal, workspacePath, observationError]);

  async function observe(
    original: MutationPromise<void>,
    settlement: () => Promise<void>,
  ) {
    setCommand({ phase: "running", original });
    try {
      await settlement();
      if (context.signal.aborted) return;
      setCommand(undefined);
      await refresh();
    } catch (error) {
      if (context.signal.aborted) return;
      if (mutationSettlementIsUnknown(error))
        setCommand({ phase: "unknown", original, error: failureText(error) });
      else {
        setCommand(undefined);
        setCommandError(failureText(error));
      }
    }
  }
  function trust(trusted: boolean) {
    const root = read.accepted?.result.repositoryRoot;
    if (!root || command || context.signal.aborted) return;
    setCommandError(undefined);
    try {
      const original = context.track(() =>
        context.runtime.hooks.setTrust(root, trusted),
      );
      void observe(original, () => original);
    } catch (error) {
      setCommandError(failureText(error));
    }
  }
  const snapshot = read.accepted?.result;
  return (
    <>
      <Toolbar
        label={t("title")}
        actions={
          <IconButton
            icon={Refresh}
            aria-label={t("refresh")}
            pending={read.loading}
            onClick={() => {
              setObservationError(undefined);
              void refresh();
            }}
          />
        }
      >
        {null}
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
            onClick={() =>
              void observe(command.original, () =>
                context.track(() =>
                  command.original.retry({ signal: context.signal }),
                ),
              )
            }
          >
            {t("command.reconcile")}
          </Button>
        </div>
      )}
      {snapshot &&
        (snapshot.hooks.length ? (
          sourceGroups(snapshot.hooks).map((group, index, groups) => (
            <PanelSection
              key={`${index}:${group.source}`}
              title={t(`scope.${group.scope}`)}
              description={<code style={literal}>{group.source}</code>}
            >
              <RecordGroup
                variant="list"
                aria-busy={
                  group.scope === "repository"
                    ? command?.phase === "running"
                    : undefined
                }
              >
                {snapshot.repositoryRoot &&
                  groups.findIndex(({ scope }) => scope === "repository") ===
                    index && (
                    <ListRow data-focus-unit="">
                      <RecordHeading
                        title={t("trust")}
                        description={t("trust.sub")}
                        actions={
                          <Switch
                            checked={snapshot.repositoryTrusted}
                            disabled={Boolean(command) || read.loading}
                            onCheckedChange={trust}
                            ariaLabel={t("trust.aria")}
                          />
                        }
                      />
                    </ListRow>
                  )}
                {group.hooks.map((value, row) => (
                  <HookRow key={`${value.event}:${row}`} value={value} t={t} />
                ))}
              </RecordGroup>
            </PanelSection>
          ))
        ) : (
          <EmptyState title={t("empty")} sub={t("empty.sub")} />
        ))}
    </>
  );
}

// Runtime lists rules in their captured order, so consecutive rules share a file.
function sourceGroups(hooks: readonly HookInfo[]) {
  const groups: { source: string; scope: HookScope; hooks: HookInfo[] }[] = [];
  for (const hook of hooks) {
    const last = groups.at(-1);
    if (last?.source === hook.source) last.hooks.push(hook);
    else groups.push({ source: hook.source, scope: hook.scope, hooks: [hook] });
  }
  return groups;
}

function HookRow({ value, t }: { value: HookInfo; t: Translate }) {
  return (
    <ListRow>
      <RecordHeading
        title={value.event}
        status={
          <>
            {value.matcher && (
              <RecordMeta>
                <code title={t("matcher")} style={literal}>
                  {value.matcher}
                </code>
              </RecordMeta>
            )}
            {value.inject !== undefined && (
              <RecordMeta>{t("kind.inject")}</RecordMeta>
            )}
            {value.failurePolicy === "required" && (
              <Badge tone="warning">{t("failure.required")}</Badge>
            )}
          </>
        }
        description={
          <code style={literal}>{value.command || value.inject}</code>
        }
      />
    </ListRow>
  );
}

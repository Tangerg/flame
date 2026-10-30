import { captureClientTelemetry } from "@/lib/observability/clientTelemetry";
import {
  Component,
  Suspense,
  useMemo,
  useState,
  useSyncExternalStore,
  type ErrorInfo,
  type ReactNode,
} from "react";
import { useLocale, useT } from "@/lib/i18n/index";
import { Button } from "@flame/ui/button";
import { FailureDetails } from "@/ui/index";
import { failureDiagnostics } from "@/lib/diagnostics";
import {
  type CapturedFile,
  type CapturedSession,
  type CapturedToolResult,
  type ComponentContext,
  type WorkspaceContext,
  type ComponentComposition,
  type ComponentReference,
  type PluginComponent,
  type SaveFileRequest,
} from "@flame/component-sdk";
import {
  isWorkspaceResolving,
  resolvedWorkspaceCwd,
  useActiveSessionId,
  useActiveSessionWorkspace,
} from "@/features/agent/public/session";
import { tupleKey } from "@/lib/tupleKey";
import { useWorkspaceFileFocus } from "@/features/workspace/public/navigation";
import { useAgent, type AgentAccess } from "@/features/agent/public/agent";
import {
  useNotifications,
  type NotificationAccess,
} from "@/features/notifications/public/notifier";
import { type FlameClient } from "@flame/runtime-client";
import {
  type PluginClientComponent,
  type PluginOperationBinding,
} from "@flame/runtime-client/wire";
import { trackContributionIO } from "@/extensions/contributionIO";
import { type ContributionLifetime } from "@/extensions/definePlugin";
import { type RuntimeConnectionGeneration } from "@/features/connection/public/services";
import { useSubmitCapturedAgentInput } from "@/features/agent/public/input";
import {
  captureToolInteraction,
  type SubmitCapturedAgentInput,
} from "@/features/plugins/adapters/toolInteraction";
import { createClientCommandRegistration } from "@/features/plugins/adapters/clientCommands";
import {
  createComponentNotifications,
  createComponentSettlementSubscription,
} from "@/features/plugins/adapters/clientNotifications";
export function componentFailureScope(
  component: ComponentReference,
  generation: RuntimeConnectionGeneration,
): string {
  return `installed component\ninstallation: ${component.installationId}\nrelease: ${component.digest}\ncomponent: ${component.componentId}\nruntime generation: ${generation.processGeneration}`;
}

type ComponentFailure = { error: unknown; diagnostics?: string };

export class ComponentBoundary extends Component<
  {
    children: ReactNode;
    scope: string;
    fallback?: ReactNode;
    resetKey?: unknown;
    onFailure?: () => void;
    reload?: () => Promise<void>;
  },
  { failure: ComponentFailure | null; resetKey: unknown }
> {
  override state: { failure: ComponentFailure | null; resetKey: unknown } = {
    failure: null,
    resetKey: this.props.resetKey,
  };
  static getDerivedStateFromProps(props: { resetKey?: unknown }, state: { resetKey: unknown }) {
    return props.resetKey !== state.resetKey ? { failure: null, resetKey: props.resetKey } : null;
  }
  static getDerivedStateFromError(error: unknown) {
    return { failure: { error } };
  }
  override componentDidCatch(error: unknown, info: ErrorInfo) {
    this.setState({
      failure: {
        error,
        diagnostics: [
          failureDiagnostics(this.props.scope, error),
          `react component stack: ${info.componentStack ?? "unavailable"}`,
        ].join("\n"),
      },
    });
    this.props.onFailure?.();
  }
  override render() {
    return this.state.failure ? (
      <>
        {this.props.fallback}
        <ComponentRecovery failure="render" reload={this.props.reload} />
        {this.state.failure.diagnostics && (
          <FailureDetails diagnostics={this.state.failure.diagnostics} />
        )}
      </>
    ) : (
      this.props.children
    );
  }
}

export function ComponentLoading() {
  const t = useT();
  return <p role="status">{t("packages.module.loading")}</p>;
}

function ComponentRecovery({
  failure,
  reload,
}: {
  failure: "render" | "connection";
  reload?: () => Promise<void>;
}) {
  const t = useT();
  const [pending, setPending] = useState(false);
  const [diagnostics, setDiagnostics] = useState<string>();
  const retry = async () => {
    if (!reload || pending) return;
    setPending(true);
    try {
      await reload();
    } catch (error) {
      setDiagnostics(failureDiagnostics("installed component reload", error));
    } finally {
      setPending(false);
    }
  };
  return (
    <div role="alert">
      <p>{t(failure === "render" ? "packages.module.failed" : "packages.module.unavailable")}</p>
      {reload && (
        <Button
          pending={pending}
          onClick={() => {
            void retry();
          }}
        >
          {t("packages.module.reload")}
        </Button>
      )}
      {diagnostics && <FailureDetails diagnostics={diagnostics} />}
    </div>
  );
}

type CapturedContext = Omit<ComponentContext, "locale" | "compose">;

class ComponentInstance {
  private controller = new AbortController();
  private listeners = new Set<() => void>();
  private context: CapturedContext;
  constructor(private readonly sources: ComponentSources) {
    this.context = this.capture();
  }
  private capture(): CapturedContext {
    const signal = AbortSignal.any([
      this.controller.signal,
      this.sources.lifetime.signal,
      this.sources.resources,
    ]);
    const track: ComponentContext["track"] = (operation) =>
      trackContributionIO(this.sources.lifetime, () => operation(signal));
    const openSession = this.sources.openSession;
    const component = Object.freeze({ ...this.sources.component });
    return Object.freeze({
      runtime: this.sources.runtime,
      component,
      sessionId: this.sources.sessionTarget?.id ?? this.sources.sessionId,
      sessionTarget: this.sources.sessionTarget
        ? Object.freeze({ ...this.sources.sessionTarget })
        : undefined,
      toolResult: this.sources.toolResult,
      interaction:
        this.sources.toolResult && this.sources.submitInput
          ? captureToolInteraction(
              this.sources.runtime,
              this.sources.submitInput,
              { runId: this.sources.toolResult.runId, itemId: this.sources.toolResult.id },
              signal,
              track,
            )
          : undefined,
      file: this.sources.file
        ? Object.freeze({
            ...this.sources.file,
            workspace: this.sources.file.workspace
              ? Object.freeze({ ...this.sources.file.workspace })
              : undefined,
          })
        : undefined,
      workspace: this.sources.workspace
        ? Object.freeze({
            ref: Object.freeze({ ...this.sources.workspace.ref }),
            focus: this.sources.workspace.focus
              ? Object.freeze({ ...this.sources.workspace.focus })
              : undefined,
          })
        : undefined,
      binding: this.sources.binding ? structuredClone(this.sources.binding) : undefined,
      openSession: openSession
        ? (sessionId: string) => {
            signal.throwIfAborted();
            openSession(sessionId);
          }
        : undefined,
      registerCommand:
        this.sources.placement === "clientEffect"
          ? createClientCommandRegistration(
              this.sources.lifetime,
              signal,
              component,
              this.sources.captureSessionId,
            )
          : () => {
              throw new Error("Client command registration requires clientEffect placement.");
            },
      subscribeRootRunSettlements:
        this.sources.placement === "clientEffect"
          ? createComponentSettlementSubscription(
              this.sources.lifetime,
              signal,
              component,
              this.sources.subscribeRootRunSettlements,
            )
          : () => {
              throw new Error("Root run observation requires clientEffect placement.");
            },
      notifications: createComponentNotifications(
        this.sources.lifetime,
        signal,
        component,
        this.sources.notificationAccess,
      ),
      saveFile: (request: SaveFileRequest) => {
        signal.throwIfAborted();
        return track(() => this.sources.saveFile(request));
      },
      signal,
      telemetry: captureClientTelemetry(signal),
      track,
    });
  }
  retire = () => this.controller.abort();
  getSnapshot = () => (this.context.signal.aborted ? null : this.context);
  subscribe = (listener: () => void) => {
    if (this.controller.signal.aborted) {
      this.controller = new AbortController();
      this.context = this.capture();
    }
    this.listeners.add(listener);
    this.context.signal.addEventListener("abort", listener, { once: true });
    listener();
    return () => {
      this.context.signal.removeEventListener("abort", listener);
      this.listeners.delete(listener);
      if (!this.listeners.size) this.controller.abort();
    };
  };
}

function RenderingInstance({
  entry: Entry,
  instance,
  input,
  contribution,
  children,
  fallback,
  reload,
  locale,
  compose,
}: ComponentProps & { instance: ComponentInstance; locale: string }) {
  const captured = useSyncExternalStore(instance.subscribe, instance.getSnapshot);
  const context = useMemo(() => {
    if (!captured) return null;
    const value: ComponentContext = Object.freeze({
      ...captured,
      locale,
      compose: (composition: ComponentComposition) => {
        captured.signal.throwIfAborted();
        return compose(value, composition);
      },
    });
    return value;
  }, [captured, locale, compose]);
  return context ? (
    <Entry context={context} input={input} contribution={contribution}>
      {children}
    </Entry>
  ) : (
    <>
      {fallback}
      <ComponentRecovery failure="connection" reload={reload} />
    </>
  );
}

export function RenderingComponent(
  props: ComponentProps &
    Omit<
      ComponentSources,
      "captureSessionId" | "notificationAccess" | "subscribeRootRunSettlements"
    >,
) {
  const locale = useLocale();
  const {
    selectAgentSession: openSession,
    getActiveSessionId: captureSessionId,
    subscribeRootRunSettlements,
  } = useAgent();
  const notificationAccess = useNotifications();
  const {
    runtime,
    binding,
    lifetime,
    resources,
    workspace,
    sessionId,
    file,
    toolResult,
    submitInput,
    sessionTarget,
    component,
    compose,
    saveFile,
    placement,
  } = props;
  const instance = useMemo(
    () =>
      new ComponentInstance({
        runtime,
        binding,
        lifetime,
        resources,
        workspace,
        sessionId,
        file,
        toolResult,
        submitInput,
        sessionTarget,
        component,
        compose,
        saveFile,
        openSession,
        captureSessionId,
        placement,
        notificationAccess,
        subscribeRootRunSettlements,
      }),
    [
      runtime,
      binding,
      lifetime,
      resources,
      workspace,
      sessionId,
      file,
      toolResult,
      submitInput,
      sessionTarget,
      component,
      compose,
      saveFile,
      openSession,
      captureSessionId,
      placement,
      notificationAccess,
      subscribeRootRunSettlements,
    ],
  );
  return (
    <ComponentBoundary
      scope={componentFailureScope(component, props.generation)}
      fallback={props.fallback}
      resetKey={instance}
      onFailure={instance.retire}
      reload={props.reload}
    >
      <Suspense fallback={props.fallback ?? <ComponentLoading />}>
        <RenderingInstance {...props} instance={instance} locale={locale} />
      </Suspense>
    </ComponentBoundary>
  );
}

type ComponentProps = {
  placement?: PluginClientComponent["placement"];
  reload?: () => Promise<void>;
  fallback?: ReactNode;
  entry: PluginComponent;
  component: ComponentReference;
  generation: RuntimeConnectionGeneration;
  compose: (context: ComponentContext, composition: ComponentComposition) => ReactNode;
  saveFile: ComponentContext["saveFile"];
  input?: unknown;
  contribution?: unknown;
  children?: ReactNode;
  runtime: FlameClient;
  binding: PluginOperationBinding | undefined;
  lifetime: ContributionLifetime;
  resources: AbortSignal;
};

type ComponentSources = Pick<
  ComponentProps,
  | "runtime"
  | "binding"
  | "lifetime"
  | "resources"
  | "component"
  | "compose"
  | "saveFile"
  | "placement"
> & {
  workspace?: WorkspaceContext;
  sessionId?: string;
  file?: CapturedFile;
  toolResult?: CapturedToolResult;
  submitInput?: SubmitCapturedAgentInput;
  sessionTarget?: CapturedSession;
  openSession?: (sessionId: string) => void;
  captureSessionId: () => string | undefined;
  notificationAccess: NotificationAccess;
  subscribeRootRunSettlements: AgentAccess["subscribeRootRunSettlements"];
};

function WorkspaceComponent(props: ComponentProps) {
  const t = useT();
  const selection = useActiveSessionWorkspace();
  const focus = useWorkspaceFileFocus();
  const cwd = resolvedWorkspaceCwd(selection);
  const workspace = useMemo(
    () =>
      cwd
        ? { ref: { path: cwd }, focus: { path: focus.path, revision: focus.revision } }
        : undefined,
    [cwd, focus.path, focus.revision],
  );
  if (!workspace)
    return (
      <p role="status">
        {isWorkspaceResolving(selection)
          ? t("packages.module.resolvingWorkspace")
          : t("packages.module.selectWorkspace")}
      </p>
    );
  return (
    <RenderingComponent
      key={tupleKey(workspace.ref.path, workspace.focus.revision.toString(), workspace.focus.path)}
      {...props}
      workspace={workspace}
    />
  );
}

function ClientComponent(props: ComponentProps) {
  const selection = useActiveSessionWorkspace();
  const sessionId = useActiveSessionId();
  const cwd = resolvedWorkspaceCwd(selection);
  const workspace = useMemo<WorkspaceContext | undefined>(
    () => (cwd ? { ref: { path: cwd } } : undefined),
    [cwd],
  );
  return (
    <RenderingComponent
      key={JSON.stringify([sessionId, cwd])}
      {...props}
      workspace={workspace}
      sessionId={sessionId ?? undefined}
    />
  );
}

function SessionComponent(props: ComponentProps) {
  const t = useT();
  const sessionId = useActiveSessionId();
  if (!sessionId) return <p role="status">{t("packages.module.selectSession")}</p>;
  return <RenderingComponent key={sessionId} {...props} sessionId={sessionId} />;
}

export function InstalledComponent({ scope, ...props }: ComponentProps & { scope: string }) {
  if (scope === "client") return <ClientComponent {...props} />;
  if (scope === "session") return <SessionComponent {...props} />;
  if (scope === "workspace") return <WorkspaceComponent {...props} />;
  if (scope === "installation") return <RenderingComponent {...props} />;
  throw new Error("The component context scope is unsupported.");
}

// A renderer receives the exact accepted read; navigation never changes its data.
export function InstalledFileComponent({
  file,
  ...props
}: ComponentProps & { file: CapturedFile }) {
  return <RenderingComponent {...props} file={file} />;
}

// Settled historical results are data; no producer code or navigation is captured.
export function InstalledToolResultComponent({
  toolResult,
  ...props
}: ComponentProps & { toolResult: CapturedToolResult }) {
  const submitInput = useSubmitCapturedAgentInput();
  return <RenderingComponent {...props} toolResult={toolResult} submitInput={submitInput} />;
}

export function InstalledSessionActionComponent({
  session,
  ...props
}: ComponentProps & { session: CapturedSession }) {
  return <RenderingComponent {...props} sessionTarget={session} />;
}

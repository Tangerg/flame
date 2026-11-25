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
  type CapturedComposer,
  type ComposerReference,
  type CapturedProject,
  type CapturedSession,
  type CapturedToolResult,
  type CapturedMessage,
  type ComponentContext,
  type WorkspaceContext,
  type ComponentComposition,
  type ComponentReference,
  type ComponentViews,
  type PluginComponent,
  type SaveFileRequest,
} from "@flame/component-sdk";
import { useOptionalWorkspace, type WorkspaceAccess } from "@/features/workspace/public/workspace";
import { packageScopedId } from "@/features/plugins/adapters/packages";
import {
  isWorkspaceResolving,
  resolvedWorkspaceCwd,
  useActiveSessionId,
  useActiveSessionWorkspace,
} from "@/features/agent/public/session";
import { tupleKey } from "@/lib/tupleKey";
import { useWorkspaceFileFocus } from "@/features/workspace/public/navigation";
import { useAgent, type AgentAccess } from "@/features/agent/public/agent";
import { useActiveSessionProject } from "@/features/projects/public/projects";
import {
  useNotifications,
  type NotificationAccess,
} from "@/features/notifications/public/notifier";
import { type FlameClient } from "@flame/runtime-client";
import {
  type ComponentScope,
  type PluginClientComponent,
  type PluginOperationBinding,
} from "@flame/runtime-client/wire";
import { trackContributionIO } from "@/extensions/contributionIO";
import { type ContributionScope } from "@/extensions/definePlugin";
import { type RuntimeConnectionGeneration } from "@/features/connection/public/services";
import { useSubmitCapturedAgentInput } from "@/features/agent/public/input";
import { type ComposerAccess, useComposer } from "@/features/composer/public/composer";
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
    // Effects and toolbar actions have no surface to recover in, so their
    // failure goes to plugin diagnostics instead of displacing the host layout.
    report?: (error: unknown, diagnostics: string) => void;
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
    const diagnostics = [
      failureDiagnostics(this.props.scope, error),
      `react component stack: ${info.componentStack ?? "unavailable"}`,
    ].join("\n");
    this.setState({ failure: { error, diagnostics } });
    this.props.report?.(error, diagnostics);
    this.props.onFailure?.();
  }
  override render() {
    if (this.state.failure && this.props.report) return null;
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

// The draft is captured with the context, so a later Session switch cannot
// redirect a component's text into another conversation.
function captureComposer(composer: ComposerAccess, signal: AbortSignal): CapturedComposer {
  const add = composer.captureAddReference();
  return Object.freeze({
    addReference({ label, text }: ComposerReference) {
      signal.throwIfAborted();
      if (!label.trim() || /[\r\n]/.test(label))
        throw new Error("Reference label must be one line.");
      if (!text.trim()) throw new Error("Reference text is empty.");
      return add({ label, text });
    },
  });
}

function capturedFile(file: CapturedFile): CapturedFile {
  return Object.freeze({
    ...file,
    workspace: file.workspace ? Object.freeze({ ...file.workspace }) : undefined,
  });
}

function capturedWorkspace(workspace: WorkspaceContext): WorkspaceContext {
  return Object.freeze({
    ref: Object.freeze({ ...workspace.ref }),
    focus: workspace.focus ? Object.freeze({ ...workspace.focus }) : undefined,
  });
}

type ViewNavigation = Pick<
  WorkspaceAccess["navigation"],
  "openViewInDock" | "viewHref" | "subscribeViewTarget"
>;

// A component opens, links and follows only its own plugin's views.
function captureViews(
  navigation: ViewNavigation,
  component: ComponentReference,
  signal: AbortSignal,
): ComponentViews {
  const view = (componentId: string) => packageScopedId(component.installationId, componentId);
  return Object.freeze({
    open(componentId: string, target?: string, origin?: HTMLElement) {
      signal.throwIfAborted();
      navigation.openViewInDock(view(componentId), origin, target);
    },
    href: (componentId: string, target?: string) => navigation.viewHref(view(componentId), target),
    subscribeTarget(listener: (target: string) => void) {
      signal.throwIfAborted();
      const stop = navigation.subscribeViewTarget(view(component.componentId), (target) => {
        if (!signal.aborted) listener(target);
      });
      signal.addEventListener("abort", stop, { once: true });
      return () => {
        signal.removeEventListener("abort", stop);
        stop();
      };
    },
  });
}

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
      this.sources.lifetime.lifetime.signal,
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
      message: this.sources.message,
      toolResult: this.sources.toolResult,
      composer: this.sources.composer ? captureComposer(this.sources.composer, signal) : undefined,
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
      file: this.sources.file && capturedFile(this.sources.file),
      workspace: this.sources.workspace && capturedWorkspace(this.sources.workspace),
      project: this.sources.project ? Object.freeze({ ...this.sources.project }) : undefined,
      binding: this.sources.binding ? structuredClone(this.sources.binding) : undefined,
      openSession: openSession
        ? (sessionId: string) => {
            signal.throwIfAborted();
            openSession(sessionId);
          }
        : undefined,
      views: this.sources.views && captureViews(this.sources.views, component, signal),
      ...this.effectCapabilities(signal, component),
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
  // Only client effects register commands and observe root Run settlements.
  private effectCapabilities(
    signal: AbortSignal,
    component: ComponentContext["component"],
  ): Pick<CapturedContext, "registerCommand" | "subscribeRootRunSettlements"> {
    const placement = this.sources.placement;
    if (placement !== "clientEffect" && placement !== "sharedClientEffect")
      return {
        registerCommand: () => {
          throw new Error("Client command registration requires clientEffect placement.");
        },
        subscribeRootRunSettlements: () => {
          throw new Error("Root run observation requires clientEffect placement.");
        },
      };
    return {
      registerCommand: createClientCommandRegistration(
        this.sources.lifetime,
        signal,
        component,
        this.sources.captureSessionId,
        this.sources.notificationAccess.messages,
      ),
      subscribeRootRunSettlements: createComponentSettlementSubscription(
        this.sources.lifetime,
        signal,
        component,
        this.sources.subscribeRootRunSettlements,
      ),
    };
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
}: InstalledComponentProps & { instance: ComponentInstance; locale: string }) {
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
  props: InstalledComponentProps &
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
  const views = useOptionalWorkspace()?.navigation;
  const {
    runtime,
    binding,
    lifetime,
    resources,
    workspace,
    project,
    sessionId,
    file,
    message,
    toolResult,
    submitInput,
    composer,
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
        project,
        sessionId,
        file,
        message,
        toolResult,
        submitInput,
        composer,
        sessionTarget,
        component,
        compose,
        saveFile,
        openSession,
        views,
        captureSessionId,
        placement,
        notificationAccess,
        subscribeRootRunSettlements,
      }),
    [
      views,
      runtime,
      binding,
      lifetime,
      resources,
      workspace,
      project,
      sessionId,
      file,
      message,
      toolResult,
      submitInput,
      composer,
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

export type InstalledComponentProps = {
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
  lifetime: ContributionScope;
  resources: AbortSignal;
};

type ComponentSources = Pick<
  InstalledComponentProps,
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
  project?: CapturedProject;
  sessionId?: string;
  message?: CapturedMessage;
  file?: CapturedFile;
  toolResult?: CapturedToolResult;
  submitInput?: SubmitCapturedAgentInput;
  composer?: ComposerAccess;
  sessionTarget?: CapturedSession;
  openSession?: (sessionId: string) => void;
  views?: ViewNavigation;
  captureSessionId: () => string | undefined;
  notificationAccess: NotificationAccess;
  subscribeRootRunSettlements: AgentAccess["subscribeRootRunSettlements"];
};

function WorkspaceComponent(props: InstalledComponentProps) {
  const t = useT();
  const composer = useComposer();
  const selection = useActiveSessionWorkspace();
  const focus = useWorkspaceFileFocus();
  const active = useActiveSessionProject();
  const cwd = resolvedWorkspaceCwd(selection);
  const workspace = useMemo(
    () =>
      cwd
        ? { ref: { path: cwd }, focus: { path: focus.path, revision: focus.revision } }
        : undefined,
    [cwd, focus.path, focus.revision],
  );
  if (!workspace || active.pending)
    return (
      <p role="status">
        {isWorkspaceResolving(selection) || active.pending
          ? t("packages.module.resolvingWorkspace")
          : t("packages.module.selectWorkspace")}
      </p>
    );
  // A workspace action must remain a return target when opening a new file focus.
  const key =
    props.placement === "workspaceChangesAction"
      ? workspace.ref.path
      : tupleKey(workspace.ref.path, workspace.focus.revision.toString(), workspace.focus.path);
  return (
    <RenderingComponent
      key={tupleKey(key, active.project?.id ?? "")}
      {...props}
      workspace={workspace}
      project={active.project}
      composer={composer}
    />
  );
}

function ClientComponent(props: InstalledComponentProps) {
  const composer = useComposer();
  const selection = useActiveSessionWorkspace();
  const sessionId = useActiveSessionId();
  const active = useActiveSessionProject();
  const cwd = resolvedWorkspaceCwd(selection);
  const workspace = useMemo<WorkspaceContext | undefined>(
    () => (cwd ? { ref: { path: cwd } } : undefined),
    [cwd],
  );
  if (active.pending) return props.fallback ?? <ComponentLoading />;
  return (
    <RenderingComponent
      key={JSON.stringify([sessionId, cwd, active.project?.id ?? null])}
      {...props}
      workspace={workspace}
      project={active.project}
      sessionId={sessionId ?? undefined}
      composer={composer}
    />
  );
}

function SessionComponent(props: InstalledComponentProps) {
  const t = useT();
  const composer = useComposer();
  const sessionId = useActiveSessionId();
  if (!sessionId) return <p role="status">{t("packages.module.selectSession")}</p>;
  return (
    <RenderingComponent key={sessionId} {...props} sessionId={sessionId} composer={composer} />
  );
}

export function InstalledComponent({
  scope,
  ...props
}: InstalledComponentProps & { scope: ComponentScope }) {
  switch (scope) {
    case "client":
      return <ClientComponent {...props} />;
    case "session":
      return <SessionComponent {...props} />;
    case "workspace":
      return <WorkspaceComponent {...props} />;
    case "installation":
      return <RenderingComponent {...props} />;
    // These render only from a target the host captured, never from a placement.
    case "file":
    case "toolResult":
      throw new Error("The component context scope is unsupported.");
  }
}

// A renderer receives the exact accepted read; navigation never changes its data.
export function InstalledFileComponent({
  file,
  ...props
}: InstalledComponentProps & { file: CapturedFile }) {
  return <RenderingComponent {...props} file={file} />;
}

// Settled historical results are data; no producer code or navigation is captured.
export function InstalledToolResultComponent({
  toolResult,
  ...props
}: InstalledComponentProps & { toolResult: CapturedToolResult }) {
  const submitInput = useSubmitCapturedAgentInput();
  return <RenderingComponent {...props} toolResult={toolResult} submitInput={submitInput} />;
}

export function InstalledSessionActionComponent({
  session,
  ...props
}: InstalledComponentProps & { session: CapturedSession }) {
  return <RenderingComponent {...props} sessionTarget={session} />;
}

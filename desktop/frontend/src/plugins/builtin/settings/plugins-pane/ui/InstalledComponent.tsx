import { captureClientTelemetry } from "@/lib/observability/clientTelemetry";
import {
  Component,
  Suspense,
  useMemo,
  useSyncExternalStore,
  type ComponentType,
  type ReactNode,
} from "react";
import { useT } from "@/lib/i18n";
import { Button } from "@flame/ui/button";
import type { CapturedFile, ComponentContext, WorkspaceContext } from "@flame/plugin-sdk";
import {
  useActiveSessionId,
  useActiveSessionWorkspace,
} from "@/plugins/builtin/agent/public/session";
import { useWorkspaceFileFocus } from "@/plugins/builtin/workspace/public/navigation";
import type { FlameClient } from "@flame/runtime-contract/client";
import type { PluginOperationBinding } from "@flame/runtime-contract/wire";
import { trackContributionIO } from "@/plugins/sdk/contributionIO";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";

export class ComponentBoundary extends Component<
  { children: ReactNode; fallback?: ReactNode },
  { failed: boolean }
> {
  override state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  override render() {
    return this.state.failed ? (
      <>
        {this.props.fallback}
        <ComponentRecovery failure="render" />
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

function ComponentRecovery({ failure }: { failure: "render" | "connection" }) {
  const t = useT();
  return (
    <div role="alert">
      <p>{t(failure === "render" ? "packages.module.failed" : "packages.module.unavailable")}</p>
      <Button onClick={() => location.reload()}>{t("packages.module.reload")}</Button>
    </div>
  );
}

class ComponentInstance {
  private controller = new AbortController();
  private listeners = new Set<() => void>();
  private context: ComponentContext;
  constructor(
    private runtime: FlameClient,
    private binding: PluginOperationBinding | undefined,
    private lifetime: ContributionLifetime,
    private resources: AbortSignal,
    private workspace?: WorkspaceContext,
    private sessionId?: string,
    private file?: CapturedFile,
  ) {
    this.context = this.capture();
  }
  private capture(): ComponentContext {
    const signal = AbortSignal.any([this.controller.signal, this.lifetime.signal, this.resources]);
    return Object.freeze({
      runtime: this.runtime,
      sessionId: this.sessionId,
      file: this.file
        ? Object.freeze({
            ...this.file,
            workspace: this.file.workspace ? Object.freeze({ ...this.file.workspace }) : undefined,
          })
        : undefined,
      workspace: this.workspace
        ? Object.freeze({
            ref: Object.freeze({ ...this.workspace.ref }),
            focus: this.workspace.focus ? Object.freeze({ ...this.workspace.focus }) : undefined,
          })
        : undefined,
      binding: this.binding ? structuredClone(this.binding) : undefined,
      signal,
      telemetry: captureClientTelemetry(signal),
      track: <T,>(operation: Promise<T>): Promise<T> =>
        trackContributionIO(this.lifetime, operation),
    });
  }
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

function RenderingComponent({
  entry,
  runtime,
  binding,
  lifetime,
  resources,
  workspace,
  sessionId,
  file,
  fallback,
}: ComponentProps & { workspace?: WorkspaceContext; sessionId?: string; file?: CapturedFile }) {
  const instance = useMemo(
    () => new ComponentInstance(runtime, binding, lifetime, resources, workspace, sessionId, file),
    [runtime, binding, lifetime, resources, workspace, sessionId, file],
  );
  const context = useSyncExternalStore(instance.subscribe, instance.getSnapshot);
  const Entry = entry;
  return (
    <ComponentBoundary fallback={fallback}>
      <Suspense fallback={fallback ?? <ComponentLoading />}>
        {context ? (
          <Entry context={context} />
        ) : (
          <>
            {fallback}
            <ComponentRecovery failure="connection" />
          </>
        )}
      </Suspense>
    </ComponentBoundary>
  );
}

type ComponentProps = {
  fallback?: ReactNode;
  entry: ComponentType<{ context: ComponentContext }>;
  runtime: FlameClient;
  binding: PluginOperationBinding | undefined;
  lifetime: ContributionLifetime;
  resources: AbortSignal;
};

function WorkspaceComponent(props: ComponentProps) {
  const t = useT();
  const selection = useActiveSessionWorkspace();
  const focus = useWorkspaceFileFocus();
  const cwd = selection.status === "ready" ? selection.cwd : undefined;
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
        {selection.status === "resolving"
          ? t("packages.module.resolvingWorkspace")
          : t("packages.module.selectWorkspace")}
      </p>
    );
  return (
    <RenderingComponent
      key={JSON.stringify([
        workspace.ref.path,
        workspace.focus.revision.toString(),
        workspace.focus.path,
      ])}
      {...props}
      workspace={workspace}
    />
  );
}

function ClientComponent(props: ComponentProps) {
  const selection = useActiveSessionWorkspace();
  const sessionId = useActiveSessionId();
  const cwd = selection.status === "ready" ? selection.cwd : undefined;
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

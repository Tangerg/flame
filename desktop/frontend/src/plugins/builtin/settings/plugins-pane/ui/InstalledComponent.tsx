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
import type { ComponentContext, WorkspaceContext } from "@flame/plugin-sdk";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { useWorkspaceFileFocus } from "@/plugins/builtin/workspace/public/navigation";
import type { FlameClient } from "@flame/runtime-contract/client";
import type { PluginOperationBinding } from "@flame/runtime-contract/wire";
import { trackContributionIO } from "@/plugins/sdk/contributionIO";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";

export class ComponentBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  override state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  override render() {
    return this.state.failed ? <ComponentRecovery failure="render" /> : this.props.children;
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
  ) {
    this.context = this.capture();
  }
  private capture(): ComponentContext {
    return Object.freeze({
      runtime: this.runtime,
      workspace: this.workspace
        ? Object.freeze({
            ref: Object.freeze({ ...this.workspace.ref }),
            focus: Object.freeze({ ...this.workspace.focus }),
          })
        : undefined,
      binding: this.binding ? structuredClone(this.binding) : undefined,
      signal: AbortSignal.any([this.controller.signal, this.lifetime.signal, this.resources]),
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
}: ComponentProps & { workspace?: WorkspaceContext }) {
  const instance = useMemo(
    () => new ComponentInstance(runtime, binding, lifetime, resources, workspace),
    [runtime, binding, lifetime, resources, workspace],
  );
  const context = useSyncExternalStore(instance.subscribe, instance.getSnapshot);
  const Entry = entry;
  return (
    <ComponentBoundary>
      <Suspense fallback={<ComponentLoading />}>
        {context ? <Entry context={context} /> : <ComponentRecovery failure="connection" />}
      </Suspense>
    </ComponentBoundary>
  );
}

type ComponentProps = {
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
  const workspace = useMemo<WorkspaceContext | undefined>(
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

export function InstalledComponent({ scope, ...props }: ComponentProps & { scope: string }) {
  if (scope === "workspace") return <WorkspaceComponent {...props} />;
  if (scope === "installation") return <RenderingComponent {...props} />;
  throw new Error("The component context scope is unsupported.");
}

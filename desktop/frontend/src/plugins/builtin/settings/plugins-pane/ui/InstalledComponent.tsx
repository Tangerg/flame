import {
  Component,
  Suspense,
  useMemo,
  useSyncExternalStore,
  type ComponentType,
  type ReactNode,
} from "react";
import { Button } from "@flame/ui/button";
import { asyncDisposeSymbol } from "dougong";
import type { ComponentContext } from "@flame/plugin-sdk";
import type { FlameClient } from "@flame/runtime-contract/client";
import type { PluginOperationBinding } from "@flame/runtime-contract/wire";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";

export class ComponentBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  override state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  override render() {
    return this.state.failed ? (
      <div role="alert">
        <p>The plugin component could not be loaded or displayed.</p>
        <Button onClick={() => location.reload()}>Reload window</Button>
      </div>
    ) : (
      this.props.children
    );
  }
}

class ComponentInstance {
  private controller = new AbortController();
  private listeners = new Set<() => void>();
  private context: ComponentContext;
  constructor(
    private runtime: FlameClient,
    private binding: PluginOperationBinding,
    private lifetime: ContributionLifetime,
    private resources: AbortSignal,
  ) {
    this.context = this.capture();
  }
  private capture(): ComponentContext {
    return Object.freeze({
      runtime: this.runtime,
      binding: structuredClone(this.binding),
      signal: AbortSignal.any([this.controller.signal, this.lifetime.signal, this.resources]),
      track: <T,>(operation: Promise<T>): Promise<T> => {
        // The caller observes the result; the contribution owner joins I/O.
        const join = () =>
          operation.then(
            () => undefined,
            () => undefined,
          );
        const cleanup = this.lifetime.cleanup(join);
        void join().then(() => cleanup[asyncDisposeSymbol]());
        return operation;
      },
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

export function InstalledComponent({
  entry,
  runtime,
  binding,
  lifetime,
  resources,
}: {
  entry: ComponentType<{ context: ComponentContext }>;
  runtime: FlameClient;
  binding: PluginOperationBinding;
  lifetime: ContributionLifetime;
  resources: AbortSignal;
}) {
  const instance = useMemo(
    () => new ComponentInstance(runtime, binding, lifetime, resources),
    [runtime, binding, lifetime, resources],
  );
  const context = useSyncExternalStore(instance.subscribe, instance.getSnapshot);
  const Entry = entry;
  return (
    <ComponentBoundary>
      <Suspense fallback={<p role="status">Loading plugin component…</p>}>
        {context ? (
          <Entry context={context} />
        ) : (
          <div role="alert">
            <p>The plugin's resource connection is unavailable.</p>
            <Button onClick={() => location.reload()}>Reload window</Button>
          </div>
        )}
      </Suspense>
    </ComponentBoundary>
  );
}

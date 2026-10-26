import { create } from "zustand";
import { createPublicationSlot } from "@/lib/publicationSlot";
import { type ServerCapabilities, type WireFeature } from "@flame/runtime-client";
import { type RuntimeServiceSnapshot } from "@/features/connection/application/runtimeService";
import {
  createRuntimeServiceController,
  type RuntimeConnectionInspection,
  type RuntimeConnectionInspector,
  type RuntimeServiceController,
  type RuntimeServiceFailure,
} from "@/features/connection/application/runtimeService";
import { RuntimeConnectionGeneration } from "@/features/connection/public/services";
interface RuntimeConnectionState {
  connectionGeneration: RuntimeConnectionGeneration | null;
  capabilities: ServerCapabilities | null;
  service: RuntimeServiceSnapshot;
}

function initialConnectionState(): RuntimeConnectionState {
  return {
    connectionGeneration: null,
    capabilities: null,
    service: { phase: "checking", observation: null, failure: null },
  };
}

function reconnectingConnectionState(): RuntimeConnectionState {
  return {
    connectionGeneration: null,
    capabilities: null,
    service: { phase: "reconnecting", observation: null, failure: null },
  };
}

export function createRuntimeConnectionProjection() {
  const useRuntimeConnectionStore = create<RuntimeConnectionState>(() => initialConnectionState());

  function useServerFeature(feature: WireFeature): boolean {
    return (
      useRuntimeConnectionStore((state) => state.capabilities?.features[feature]?.enabled) === true
    );
  }

  function serverFeature(feature: WireFeature): boolean {
    return useRuntimeConnectionStore.getState().capabilities?.features[feature]?.enabled === true;
  }

  function runtimeSupportsStreamingMethod(method: string): boolean {
    return (
      useRuntimeConnectionStore.getState().capabilities?.streamingMethods?.includes(method) ?? false
    );
  }

  function runtimeSupportsTopic(topic: string): boolean {
    return (
      useRuntimeConnectionStore
        .getState()
        .capabilities?.runtimeTopics.some((advertised) => advertised === topic) ?? false
    );
  }

  const capabilities = {
    useCapability: useServerFeature,
    hasCapability: serverFeature,
    supportsStreamingMethod: runtimeSupportsStreamingMethod,
    supportsRuntimeTopic: runtimeSupportsTopic,
    negotiated: () => useRuntimeConnectionStore.getState().capabilities,
  };

  interface RuntimeConnectionOwner {
    connectionGeneration(): RuntimeConnectionGeneration | null;
    subscribeConnection(onChange: () => void): () => void;
    subscribeServerReplacement(onReplace: () => void): () => void;
    replaceEndpoint(commit: () => void): Promise<void>;
    reportConnectionLoss(expectedGeneration: RuntimeConnectionGeneration): void;
    dispose(): void;
  }

  class RuntimeConnectionOwnerImplementation implements RuntimeConnectionOwner {
    readonly #controller: RuntimeServiceController;

    readonly #serverReplacementListeners = new Set<() => void>();
    #disposed = false;

    constructor(inspector: RuntimeConnectionInspector<ServerCapabilities>) {
      this.#controller = createRuntimeServiceController(inspector, {
        checking: () => this.#checking(),
        replace: (inspection) => this.#replace(inspection),
        unavailable: (failure) => this.#unavailable(failure),
      });
    }

    refresh(): Promise<void> {
      return this.#ownsGeneration() ? this.#controller.refresh() : Promise.resolve();
    }

    start(): void {
      if (!this.#ownsGeneration()) return;
      this.#controller.start();
    }

    connectionGeneration(): RuntimeConnectionGeneration | null {
      return this.#ownsGeneration()
        ? useRuntimeConnectionStore.getState().connectionGeneration
        : null;
    }

    subscribeConnection(onChange: () => void): () => void {
      if (!this.#ownsGeneration()) return () => undefined;
      let current = useRuntimeConnectionStore.getState();
      return useRuntimeConnectionStore.subscribe((state) => {
        if (!this.#ownsGeneration()) return;
        if (
          state.connectionGeneration === current.connectionGeneration &&
          state.capabilities === current.capabilities
        ) {
          current = state;
          return;
        }
        current = state;
        onChange();
      });
    }

    subscribeServerReplacement(onReplace: () => void): () => void {
      if (!this.#ownsGeneration()) return () => undefined;
      this.#serverReplacementListeners.add(onReplace);
      return () => this.#serverReplacementListeners.delete(onReplace);
    }

    dispose(): void {
      if (this.#disposed) return;
      const ownsGeneration = runtimeConnectionPublication.owns(this);
      this.#disposed = true;
      this.#controller.dispose();
      this.#serverReplacementListeners.clear();
      if (!ownsGeneration) return;
      runtimeConnectionPublication.withdraw(this);
      useRuntimeConnectionStore.setState(initialConnectionState(), true);
    }

    replaceEndpoint(commit: () => void): Promise<void> {
      if (!this.#ownsGeneration()) return Promise.resolve();
      useRuntimeConnectionStore.setState(initialConnectionState(), true);
      commit();
      for (const listener of this.#serverReplacementListeners) listener();
      return this.#controller.replace();
    }

    reportConnectionLoss(expectedGeneration: RuntimeConnectionGeneration): void {
      if (!this.#ownsGeneration()) return;
      const current = useRuntimeConnectionStore.getState();
      if (current.connectionGeneration !== expectedGeneration) return;

      useRuntimeConnectionStore.setState(reconnectingConnectionState(), true);
      this.#controller.recover();
    }

    #ownsGeneration(): boolean {
      return !this.#disposed && runtimeConnectionPublication.owns(this);
    }

    #checking(): void {
      if (!this.#ownsGeneration()) return;
      useRuntimeConnectionStore.setState((state) => ({
        ...state,
        service: { ...state.service, phase: "checking", failure: null },
      }));
    }

    #replace(inspection: RuntimeConnectionInspection<ServerCapabilities>): void {
      if (!this.#ownsGeneration()) return;
      const current = useRuntimeConnectionStore.getState();
      const connectionGeneration =
        current.connectionGeneration?.belongsTo(inspection.processGeneration) === true
          ? current.connectionGeneration
          : RuntimeConnectionGeneration.forProcess(inspection.processGeneration);
      useRuntimeConnectionStore.setState({
        connectionGeneration,
        capabilities: inspection.capabilities,
        service: {
          phase: inspection.service.health,
          observation: inspection.service,
          failure: null,
        },
      });
    }

    #unavailable(failure: RuntimeServiceFailure): void {
      if (!this.#ownsGeneration()) return;
      useRuntimeConnectionStore.setState({
        connectionGeneration: null,
        capabilities: null,
        service: { phase: "unavailable", observation: null, failure },
      });
    }
  }

  const runtimeConnectionPublication =
    createPublicationSlot<RuntimeConnectionOwnerImplementation>();

  function startRuntimeConnection(
    inspector: RuntimeConnectionInspector<ServerCapabilities>,
  ): RuntimeConnectionOwner {
    const owner = new RuntimeConnectionOwnerImplementation(inspector);
    runtimeConnectionPublication.publish(owner, (predecessor) => predecessor.dispose());
    useRuntimeConnectionStore.setState(initialConnectionState(), true);
    owner.start();
    return owner;
  }

  function dispose(): void {
    runtimeConnectionPublication.current()?.dispose();
    useRuntimeConnectionStore.setState(initialConnectionState(), true);
  }

  const status = {
    useSnapshot: () => useRuntimeConnectionStore((state) => state.service),
    snapshot: () => useRuntimeConnectionStore.getState().service,
    refresh: () => runtimeConnectionPublication.current()?.refresh() ?? Promise.resolve(),
  };
  const current = () => {
    const owner = runtimeConnectionPublication.current();
    if (!owner) throw new Error("Runtime connection is not ready");
    return owner;
  };
  const stream = {
    connectionGeneration: () =>
      runtimeConnectionPublication.current()?.connectionGeneration() ?? null,
    subscribeConnection: (onChange: () => void) => current().subscribeConnection(onChange),
    reportConnectionLoss: (expected: RuntimeConnectionGeneration) =>
      current().reportConnectionLoss(expected),
  };
  const scope = {
    subscribeReplacement: (onReplace: () => void) =>
      current().subscribeServerReplacement(onReplace),
  };
  return {
    store: useRuntimeConnectionStore,
    capabilities,
    status,
    stream,
    scope,
    start: startRuntimeConnection,
    dispose,
  };
}

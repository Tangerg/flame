import type { WorkspaceEventLoop, WorkspaceReadTarget } from "./workspaceEventLoop";
import type { RuntimeConnectionGeneration } from "@/plugins/builtin/runtime/public/services";
import { delayUntilAborted } from "@/lib/abortableDelay";

export type WorkspaceCwdResolution =
  { status: "resolved"; cwd?: string } | { status: "unavailable" };

export interface WorkspaceEventSubscriptionPorts {
  canSubscribe: () => boolean;
  connectionGeneration: () => RuntimeConnectionGeneration | null;
  subscribeConnection: (onChange: () => void) => () => void;
  retireReadModels: () => void;
  resolveWorkspaceCwd: (signal: AbortSignal) => Promise<WorkspaceCwdResolution>;
  reportResolutionError: (error: unknown) => void;
  subscribeWorkspaceCwdInputs: (onChange: (change: WorkspaceCwdInputChange) => void) => () => void;
  readTargets: () => readonly WorkspaceReadTarget[];
  subscribeReadTargets: (onChange: () => void) => () => void;
  loop: WorkspaceEventLoop;
}

class RuntimeEventLoopOwner {
  #observedConnection = false;
  #connectionGeneration: RuntimeConnectionGeneration | null = null;
  #abort: AbortController | null = null;

  constructor(
    private readonly loop: WorkspaceEventLoop,
    private readonly retireReadModels: () => void,
  ) {}

  reconcile(connectionGeneration: RuntimeConnectionGeneration | null, canSubscribe: boolean): void {
    const generationChanged =
      this.#observedConnection && this.#connectionGeneration !== connectionGeneration;
    const shouldStream = connectionGeneration !== null && canSubscribe;
    if (this.#observedConnection && !generationChanged && (this.#abort !== null) === shouldStream)
      return;

    if (generationChanged) this.retireReadModels();
    this.#abort?.abort();
    this.#abort = null;
    this.#observedConnection = true;
    this.#connectionGeneration = connectionGeneration;
    if (!shouldStream) return;

    const abort = new AbortController();
    this.#abort = abort;
    void this.loop.start(abort.signal, connectionGeneration);
  }

  dispose(): void {
    this.#abort?.abort();
    this.#abort = null;
  }
}

export type WorkspaceCwdInputChange = "identity" | "projection";

const RESOLVE_RETRY_BASE_MS = 1_000;
const RESOLVE_RETRY_CAP_MS = 30_000;

export function startWorkspaceEventSubscription(
  ports: WorkspaceEventSubscriptionPorts,
): () => void {
  const controller = new AbortController();
  const eventLoop = new RuntimeEventLoopOwner(ports.loop, ports.retireReadModels);
  let retargetLease: object = {};
  let resolutionAbort: AbortController | null = null;

  const resolveTarget = (lease: object): void => {
    resolutionAbort?.abort();
    const attemptAbort = new AbortController();
    resolutionAbort = attemptAbort;
    void (async () => {
      let attempt = 0;
      while (!attemptAbort.signal.aborted && !controller.signal.aborted) {
        try {
          const resolution = await ports.resolveWorkspaceCwd(attemptAbort.signal);
          if (lease !== retargetLease || attemptAbort.signal.aborted || controller.signal.aborted)
            return;
          if (resolution.status === "resolved") {
            const reads = ports.readTargets();
            ports.loop.retarget({
              type: "workspace",
              ...(resolution.cwd ? { cwd: resolution.cwd } : {}),
              ...(reads.length ? { reads } : {}),
            });
          } else {
            ports.loop.retarget({ type: "none" });
          }
          return;
        } catch (error) {
          if (lease !== retargetLease || attemptAbort.signal.aborted || controller.signal.aborted)
            return;
          ports.reportResolutionError(error);
          await delayUntilAborted(
            Math.min(RESOLVE_RETRY_BASE_MS * 2 ** attempt, RESOLVE_RETRY_CAP_MS),
            attemptAbort.signal,
          );
          attempt += 1;
        }
      }
    })();
  };

  const retarget = (change: WorkspaceCwdInputChange): void => {
    const lease = (retargetLease = {});
    if (change === "identity") ports.loop.retarget({ type: "none" });
    resolveTarget(lease);
  };

  const reconcileConnection = (): void => {
    if (controller.signal.aborted) return;
    eventLoop.reconcile(ports.connectionGeneration(), ports.canSubscribe());
  };

  reconcileConnection();
  const unsubscribeConnection = ports.subscribeConnection(reconcileConnection);
  retarget("identity");
  const unsubscribeCwdInputs = ports.subscribeWorkspaceCwdInputs(retarget);
  const unsubscribeReads = ports.subscribeReadTargets(() => retarget("projection"));

  return () => {
    retargetLease = {};
    resolutionAbort?.abort();
    unsubscribeConnection();
    unsubscribeCwdInputs();
    unsubscribeReads();
    eventLoop.dispose();
    controller.abort();
  };
}

import { useEffect, useMemo, useRef, useSyncExternalStore, type ReactNode } from "react";
import { asyncDisposeSymbol } from "dougong";
import { parseJSON, stringifyJSON, type FlameClient } from "@flame/runtime-client";
import {
  type PluginInstallation,
  type ResolvePluginPointRequest,
} from "@flame/runtime-client/wire";
import {
  type ComponentComposition,
  type ComponentContext,
  type ComponentReference,
  type ComponentValue,
} from "@flame/component-sdk";
import { type ContributionLifetime } from "@/extensions/definePlugin";
import { trackContributionIO } from "@/extensions/contributionIO";
import { reportPluginError } from "@/extensions/errors";
import { ASYNC_OWNERSHIP_RETIRED, settleBeforeAbort } from "@/lib/asyncOwnership";
import { usePackages } from "@/features/plugins/adapters/packages";
import {
  ComponentModules,
  type LoadedComponent,
} from "@/features/plugins/adapters/componentModules";
import {
  ComponentBoundary,
  ComponentLoading,
  RenderingComponent,
  componentFailureScope,
} from "@/features/plugins/ui/InstalledComponent";
import { type RuntimeConnectionGeneration } from "@/features/connection/public/services";
interface LoadedContribution extends LoadedComponent {
  valueSource: string;
  key: string;
  reference: ComponentReference;
  value: ComponentValue;
  binding: ComponentContext["binding"];
}

interface ResolvedComponent extends LoadedContribution {
  compose: ComponentContextRenderer;
}

type ComponentContextRenderer = (
  context: ComponentContext,
  composition: ComponentComposition,
) => ReactNode;

function identity(value: ComponentReference): string {
  return `${value.installationId}/${value.digest}/${value.componentId}`;
}

// Runtime owns contribution membership and order. This adapter only realizes
// exact component bindings and keeps the captured parent context/lifetime.
export class ComponentCompositions {
  constructor(
    private readonly client: FlameClient,
    private readonly modules: ComponentModules,
    private readonly endpoint: string,
    readonly generation: RuntimeConnectionGeneration,
    private readonly moduleScope: ContributionLifetime,
  ) {}

  renderer(
    lifetime: ContributionLifetime,
    ancestry: readonly ComponentReference[],
  ): ComponentContextRenderer {
    return (context, composition) => (
      <ComposedComponents
        host={this}
        lifetime={lifetime}
        ancestry={ancestry}
        context={context}
        composition={composition}
      />
    );
  }

  async resolve(
    request: ResolvePluginPointRequest,
    installations: readonly PluginInstallation[],
    context: ComponentContext,
    lifetime: ContributionLifetime,
    ancestry: readonly ComponentReference[],
  ): Promise<LoadedContribution[]> {
    const signal = AbortSignal.any([context.signal, lifetime.signal]);
    const page = await trackContributionIO(
      lifetime,
      this.client.plugins.resolvePoint(request, signal),
    );
    signal.throwIfAborted();
    return Promise.all(
      page.data.map(async (binding) => {
        const installation = installations.find(
          (row) =>
            row.id === binding.installationId &&
            row.selected.digest === binding.digest &&
            row.presentation === "admitted",
        );
        if (!installation) throw new Error("The composed plugin release is no longer available.");
        const value = parseJSON(binding.value) as ComponentValue;
        if (!value || typeof value !== "object" || typeof value.component !== "string")
          throw new Error("The component point value must name a local component.");
        const entry = installation.selected.components.find(
          (row) => row.id === value.component && row.placement === "export",
        );
        if (!entry)
          throw new Error(
            `The composed component "${value.component}" is not exported by its bound release.`,
          );
        const reference: ComponentReference = {
          installationId: binding.installationId,
          digest: binding.digest,
          componentId: entry.id,
        };
        if (ancestry.some((parent) => identity(parent) === identity(reference)))
          throw new Error(
            `Component composition cycle: ${[...ancestry, reference].map(identity).join(" -> ")}`,
          );
        if (
          (entry.scope === "workspace" && !context.workspace) ||
          (entry.scope === "session" && !context.sessionId) ||
          (entry.scope === "file" && !context.file) ||
          (entry.scope === "toolResult" && !context.toolResult)
        )
          throw new Error(`The composed component requires captured ${entry.scope} context.`);
        const contribution = installation.selected.contributions.find(
          (row) => row.id === entry.contribution,
        );
        if (entry.contribution && !contribution)
          throw new Error("The composed component's callable contribution is unavailable.");
        const loaded = await settleBeforeAbort(
          this.modules.component(
            this.moduleScope,
            this.client,
            this.endpoint,
            this.generation,
            installation,
            entry,
          ),
          signal,
        );
        if (loaded === ASYNC_OWNERSHIP_RETIRED) throw signal.reason;
        signal.throwIfAborted();
        return {
          ...loaded,
          valueSource: binding.value,
          key: `${binding.installationId}/${binding.digest}/${binding.contribution}`,
          reference,
          value,
          binding: contribution
            ? {
                installationId: binding.installationId,
                digest: binding.digest,
                contribution: contribution.id,
                contract: contribution.contract,
              }
            : undefined,
        };
      }),
    );
  }
}

interface CompositionProps {
  host: ComponentCompositions;
  lifetime: ContributionLifetime;
  ancestry: readonly ComponentReference[];
  context: ComponentContext;
  composition: ComponentComposition;
}

function ComposedComponents(props: CompositionProps) {
  const packages = usePackages();
  const { composition } = props;
  const requestKey = stringifyJSON({
    contract: composition.contract,
    target: composition.target,
    attributes: stringifyJSON(composition.attributes),
    order: composition.order ?? [],
  });
  const resetKey = useMemo(
    () => ({ requestKey, rows: packages.data }),
    [requestKey, packages.data],
  );
  return (
    <ComponentBoundary
      scope={componentFailureScope(props.context.component, props.host.generation)}
      resetKey={resetKey}
    >
      <InstalledComposition
        {...props}
        rows={packages.data}
        failure={packages.error}
        requestKey={requestKey}
      />
    </ComponentBoundary>
  );
}

type CompositionSnapshot =
  | { status: "loading" }
  | { status: "failed"; error: unknown }
  | {
      status: "ready";
      signature: string;
      entries: ResolvedComponent[];
      lifetime: ContributionLifetime;
    };

class CompositionInstance {
  private snapshot: CompositionSnapshot = { status: "loading" };
  private readonly listeners = new Set<() => void>();
  private readonly reads = new Map<ContributionLifetime, Promise<void>>();
  private retiring = Promise.resolve();

  constructor(
    private readonly host: ComponentCompositions,
    private readonly lifetime: ContributionLifetime,
    private readonly ancestry: readonly ComponentReference[],
    private readonly context: ComponentContext,
    private readonly requestKey: string,
  ) {}

  getSnapshot = () => this.snapshot;
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };

  private publish(snapshot: CompositionSnapshot) {
    this.snapshot = snapshot;
    for (const listener of this.listeners) listener();
  }

  private retireRendering(): Promise<void> {
    if (this.snapshot.status === "ready") {
      const current = this.snapshot;
      this.publish({ status: "loading" });
      const closing = current.lifetime[asyncDisposeSymbol]();
      this.retiring = Promise.all([this.retiring, closing]).then(() => undefined);
    }
    return this.retiring;
  }

  refresh(rows: PluginInstallation[], predecessor: Promise<void>): () => void {
    if (this.lifetime.signal.aborted || this.context.signal.aborted) return () => {};
    const read = this.lifetime.lifetime("component-point-read");
    const signal = AbortSignal.any([this.context.signal, read.signal]);
    if (
      this.snapshot.status === "ready" &&
      this.snapshot.entries.some(
        ({ reference }) =>
          !rows.some(
            (row) =>
              row.id === reference.installationId &&
              row.selected.digest === reference.digest &&
              row.presentation === "admitted",
          ),
      )
    ) {
      void this.retireRendering().catch((error: unknown) => this.report(error));
    }
    const load = async () => {
      try {
        await predecessor;
        await this.retiring;
        signal.throwIfAborted();
        const contributions = await this.host.resolve(
          JSON.parse(this.requestKey) as ResolvePluginPointRequest,
          rows,
          this.context,
          read,
          this.ancestry,
        );
        signal.throwIfAborted();
        const signature = stringifyJSON(
          contributions.map((entry) => [entry.key, entry.valueSource]),
        );
        if (this.snapshot.status === "ready" && this.snapshot.signature === signature) return;
        await this.retireRendering();
        signal.throwIfAborted();
        const rendering = this.lifetime.lifetime("component-composition");
        const entries = contributions.map((entry) => ({
          ...entry,
          compose: this.host.renderer(rendering, [...this.ancestry, entry.reference]),
        }));
        this.publish({ status: "ready", signature, entries, lifetime: rendering });
      } catch (error) {
        if (!signal.aborted) {
          let reason = error;
          try {
            await this.retireRendering();
          } catch (cleanupError) {
            reason = new AggregateError(
              [error, cleanupError],
              "Component composition failed to retire",
            );
          }
          if (!signal.aborted) this.publish({ status: "failed", error: reason });
        }
      } finally {
        try {
          await read[asyncDisposeSymbol]();
        } finally {
          this.reads.delete(read);
        }
      }
    };
    const work = trackContributionIO(this.lifetime, load());
    this.reads.set(read, work);
    void work.catch((error: unknown) => this.report(error));
    return () => {
      void read[asyncDisposeSymbol]().catch((error: unknown) => this.report(error));
    };
  }

  report(error: unknown) {
    reportPluginError(`package:${this.context.component.installationId}`, "other", error);
  }

  async dispose(): Promise<void> {
    const rendering = this.retireRendering();
    const reads = [...this.reads].map(async ([scope, work]) => {
      await Promise.all([scope[asyncDisposeSymbol](), work]);
    });
    await Promise.all([rendering, ...reads]);
  }
}

function InstalledComposition({
  host,
  lifetime,
  ancestry,
  context,
  composition,
  rows,
  failure,
  requestKey,
}: CompositionProps & {
  rows: PluginInstallation[] | undefined;
  failure: unknown;
  requestKey: string;
}) {
  const retiring = useRef(Promise.resolve());
  const instance = useMemo(
    () => new CompositionInstance(host, lifetime, ancestry, context, requestKey),
    [host, lifetime, ancestry, context, requestKey],
  );
  const result = useSyncExternalStore(instance.subscribe, instance.getSnapshot);
  useEffect(
    () => () => {
      retiring.current = instance.dispose();
      void retiring.current.catch((error: unknown) => instance.report(error));
    },
    [instance],
  );
  useEffect(() => (rows ? instance.refresh(rows, retiring.current) : undefined), [instance, rows]);
  if (failure) throw failure;
  if (result.status === "failed") throw result.error;
  if (result.status === "loading") return <ComponentLoading />;
  const descriptor = parseJSON(composition.contract) as {
    composition: "collection" | "selection" | "decoration";
  };
  const render = (entry: ResolvedComponent, children?: ReactNode) => (
    <RenderingComponent
      key={entry.key}
      entry={entry.component}
      resources={entry.signal}
      runtime={context.runtime}
      binding={entry.binding}
      lifetime={result.lifetime}
      component={entry.reference}
      generation={host.generation}
      compose={entry.compose}
      workspace={context.workspace}
      sessionId={context.sessionId}
      file={context.file}
      toolResult={context.toolResult}
      sessionTarget={context.sessionTarget}
      input={composition.input}
      contribution={entry.value.props}
    >
      {children}
    </RenderingComponent>
  );
  if (descriptor.composition === "decoration")
    return result.entries.reduceRight<ReactNode>(
      (children, entry) => render(entry, children),
      composition.children,
    );
  if (descriptor.composition === "selection")
    return result.entries.length ? render(result.entries[0]!) : composition.children;
  if (descriptor.composition === "collection")
    return (
      <>
        {composition.children}
        {result.entries.map((entry) => render(entry))}
      </>
    );
  throw new Error("The component point has an unsupported composition rule.");
}

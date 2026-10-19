import { createRoot } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { asyncDisposeSymbol } from "dougong";
import type { ComponentContext } from "@flame/plugin-sdk";
import type { FlameClient } from "@flame/runtime-contract/client";
import type {
  PluginClientComponent,
  PluginInstallation,
  ResolvePluginPointRequest,
} from "@flame/runtime-contract/wire";
import { queryClient } from "@/lib/queryClient";
import { definePlugin, startKernel, stopKernel } from "@/plugins/sdk";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import { DATA_PROVIDER } from "@/plugins/sdk/kernelPoints";
import { RuntimeConnectionGeneration } from "@/plugins/builtin/runtime/public/services";
import { ComponentModules } from "@/plugins/builtin/settings/plugins-pane/adapters/componentModules";
import { ComponentCompositions } from "@/plugins/builtin/settings/plugins-pane/ui/ComponentCompositions";
import { RenderingComponent } from "@/plugins/builtin/settings/plugins-pane/ui/InstalledComponent";
import { PACKAGES_KEY } from "@/plugins/builtin/settings/plugins-pane/application/packages";
import declaration from "../../../examples/plugins/composition/provider/io.github.tangerg.flame/declaration.json";
import frames from "../../../examples/plugins/composition/provider/io.github.tangerg.flame/contracts/frames.json";

interface Sample {
  count: number;
  moduleLoadMs: number;
  decorationDOMMs: number;
  decorationFrameMs: number;
  moduleRequests: number;
}

declare global {
  interface Window {
    compositionBaseline: {
      measure(count: number): Promise<Sample>;
      dispose(): Promise<void>;
    };
  }
}

async function setup() {
  const entry = declaration.components.find((row) => row.id === "frame") as PluginClientComponent;
  const installation: PluginInstallation = {
    id: "11111111-1111-4111-8111-111111111111",
    source: "composition-baseline-fixture",
    enabled: true,
    inputStates: {},
    disabledServers: [],
    disabledSkills: [],
    realization: { type: "available", unavailableBackends: [] },
    presentation: "admitted",
    selected: {
      digest: "a".repeat(64),
      name: "composition-baseline-provider",
      components: [entry],
      contributions: [],
      points: [],
      pointValues: [],
      servers: [],
      inputs: [],
      skills: [],
      themes: [],
      diagnostics: [],
    },
  };
  let moduleRequests = 0;
  // Metadata and lease admission are deterministic here. Go benchmarks measure
  // core resolution; this sample isolates the browser realization boundary.
  const runtime = {
    plugins: {
      openResources: async () => {
        moduleRequests++;
        const closed = Promise.withResolvers<void>();
        return {
          result: { token: "baseline", entry: entry.entry },
          closed: closed.promise,
          close: async () => closed.resolve(),
        };
      },
      resolvePoint: async (request: ResolvePluginPointRequest) => ({
        data: (request.order ?? []).map((member, index) => ({
          ...member,
          digest: installation.selected.digest,
          value: JSON.stringify({ component: "frame", props: { label: `Frame ${index + 1}` } }),
        })),
      }),
    },
  } as unknown as FlameClient;
  let lifetime!: ContributionLifetime;
  const host = await startKernel([
    definePlugin({
      name: "benchmark.composition",
      setup(ctx) {
        lifetime = ctx.lifetime("browser-modules");
        ctx.contribute(DATA_PROVIDER, { key: PACKAGES_KEY, fetcher: async () => [installation] });
      },
    }),
  ]);
  queryClient.setQueryData([PACKAGES_KEY], [installation]);
  const generation = RuntimeConnectionGeneration.forProcess("composition-baseline");
  const modules = new ComponentModules();
  const compositions = new ComponentCompositions(
    runtime,
    modules,
    location.origin,
    generation,
    lifetime,
  );
  const reference = {
    installationId: "22222222-2222-4222-8222-222222222222",
    digest: "b".repeat(64),
    componentId: "baseline-consumer",
  };
  const element = document.getElementById("root")!;
  const root = createRoot(element);
  let rendering: ContributionLifetime | undefined;
  let sequence = 0;
  function Board({ context, input }: { context: ComponentContext; input?: unknown }) {
    const count = input as number;
    return context.compose({
      contract: JSON.stringify(frames),
      target: { scope: "installation", reference: reference.installationId },
      attributes: {},
      order: Array.from({ length: count }, (_, index) => ({
        installationId: installation.id,
        contribution: `frame-${index}`,
      })),
      children: <p data-sample={sequence}>Consumer payload</p>,
    });
  }
  window.compositionBaseline = {
    async measure(count) {
      if (![1, 8, 32].includes(count)) throw new Error("baseline supports 1, 8 or 32 frames");
      if (rendering) await rendering[asyncDisposeSymbol]();
      const scope = lifetime.lifetime(`render-${++sequence}`);
      rendering = scope;
      const began = performance.now();
      await modules.component(lifetime, runtime, location.origin, generation, installation, entry);
      const loaded = performance.now();
      let rendered = loaded;
      root.render(
        <QueryClientProvider client={queryClient}>
          <RenderingComponent
            key={sequence}
            entry={Board}
            input={count}
            component={reference}
            generation={generation}
            compose={compositions.renderer(scope, [reference])}
            runtime={runtime}
            binding={undefined}
            lifetime={scope}
            resources={lifetime.signal}
          />
        </QueryClientProvider>,
      );
      await new Promise<void>((resolve, reject) => {
        const timer = setTimeout(() => {
          observer.disconnect();
          reject(new Error("decoration did not reach its expected rendered frame count"));
        }, 5_000);
        const observer = new MutationObserver(() => {
          if (
            element.querySelectorAll("section").length !== count ||
            !element.querySelector(`[data-sample="${sequence}"]`)
          )
            return;
          rendered = performance.now();
          observer.disconnect();
          clearTimeout(timer);
          requestAnimationFrame(() => requestAnimationFrame(() => resolve()));
        });
        observer.observe(element, { childList: true, subtree: true });
      });
      return {
        count,
        moduleLoadMs: loaded - began,
        decorationDOMMs: rendered - loaded,
        decorationFrameMs: performance.now() - loaded,
        moduleRequests,
      };
    },
    async dispose() {
      root.unmount();
      await stopKernel(host);
      queryClient.clear();
    },
  };
}

void setup().catch((error: unknown) => {
  document.getElementById("root")!.textContent = String(error);
  throw error;
});

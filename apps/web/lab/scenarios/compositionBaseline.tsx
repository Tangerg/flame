import type { ReactNode } from "react";
import * as stylex from "@stylexjs/stylex";
import { ScrollArea } from "@/ui/index";
import { AgentWorkspaceView } from "@/ui/agent/index";
import { type ComponentContext } from "@flame/component-sdk";
import { type PluginClientComponent, type PluginInstallation } from "@flame/runtime-client/wire";
import { definePlugin, type ContributionScope } from "@/extensions/definePlugin";
import { loadPluginsForTest } from "@/test/kernel";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { queryClient } from "@/lib/queryClient";
import { RuntimeConnectionGeneration } from "@/features/connection/public/services";
import { ComponentModules } from "@/features/plugins/adapters/componentModules";
import { ComponentCompositions } from "@/features/plugins/ui/ComponentCompositions";
import { RenderingComponent } from "@/features/plugins/ui/InstalledComponent";
import { PACKAGES_KEY } from "@/features/plugins/adapters/packages";
import { COMPOSITION_SAMPLES } from "./compositionFixtureStates";
import { createCompositionTransport } from "./compositionTransport";
import declaration from "../../../../examples/plugins/composition/provider/package/io.github.tangerg.flame/declaration.json";
import frames from "../../../../examples/plugins/composition/provider/package/io.github.tangerg.flame/contracts/frames.json";

const styles = stylex.create({
  frame: { display: "flex", height: "100dvh", minHeight: 0, minWidth: 0 },
});

export async function createCompositionFixture(
  state: string,
  wrap: (children: ReactNode) => ReactNode,
) {
  const sample = COMPOSITION_SAMPLES.find((sample) => sample.id === state);
  if (!sample) throw new Error(`Unknown composition sample: ${state}`);
  const entry = declaration.components.find((row) => row.id === "frame") as PluginClientComponent;
  const installation: PluginInstallation = {
    id: "11111111-1111-4111-8111-111111111111",
    source: "composition-lab-fixture",
    enabled: true,
    inputStates: {},
    disabledServers: [],
    disabledSkills: [],
    realization: { type: "available", unavailableBackends: [] },
    presentation: "admitted",
    selected: {
      digest: "a".repeat(64),
      name: "composition-provider",
      terminalViews: [],
      components: [entry],
      contributions: [],
      points: [],
      pointValues: [],
      servers: [],
      inputs: [],
      skills: [],
      locales: [],
      themes: [],
      diagnostics: [],
    },
  };
  const count = sample.frames;
  const transport = createCompositionTransport(installation, entry);
  const runtime = transport.client;
  const modules = new ComponentModules(wrap);
  let lifetime!: ContributionScope;
  await loadPluginsForTest(
    definePlugin({
      name: "flame.lab.composition",
      setup(ctx) {
        lifetime = ctx.contributions.child("browser-modules");
        ctx.contributions.add(DATA_PROVIDER, {
          key: PACKAGES_KEY,
          fetcher: async () => [installation],
        });
        ctx.lifetime.defer(async () => {
          try {
            await modules.close();
          } finally {
            transport.dispose();
          }
        });
      },
    }),
  );
  queryClient.setQueryData([PACKAGES_KEY], [installation]);
  const generation = RuntimeConnectionGeneration.forProcess("composition-lab");
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
    componentId: "composition-consumer",
  };
  function Board({ context }: { context: ComponentContext }) {
    return context.compose({
      contract: JSON.stringify(frames),
      target: { scope: "installation", reference: reference.installationId },
      attributes: {},
      order: Array.from({ length: count }, (_, index) => ({
        installationId: installation.id,
        contribution: `frame-${index}`,
      })),
      children: <p>Consumer payload</p>,
    });
  }
  return {
    client: runtime,
    node: (
      <div {...stylex.props(styles.frame)}>
        <AgentWorkspaceView>
          <ScrollArea>
            <RenderingComponent
              entry={Board}
              component={reference}
              generation={generation}
              compose={compositions.renderer(lifetime, [reference])}
              saveFile={async () => "requested"}
              runtime={runtime}
              binding={undefined}
              lifetime={lifetime}
              resources={lifetime.lifetime.signal}
            />
          </ScrollArea>
        </AgentWorkspaceView>
      </div>
    ),
  };
}

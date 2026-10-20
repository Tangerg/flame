import { type ComponentProps } from "react";
import { type FlameClient } from "@flame/runtime-client";
import { type PluginOperationBinding } from "@flame/runtime-client/wire";
import { type ContributionLifetime } from "@/extensions/definePlugin";
import { InstalledComponent } from "@/features/plugins/ui/InstalledComponent";
import { VISUAL_RUNTIME_GENERATION, VISUAL_PLUGIN_DIGEST } from "./agentFixtureFacts";
export function visualComponentProps(
  runtime: FlameClient,
  lifetime: ContributionLifetime,
  componentId: string,
  binding?: PluginOperationBinding,
): Omit<ComponentProps<typeof InstalledComponent>, "entry" | "scope"> {
  return {
    runtime,
    lifetime,
    resources: lifetime.signal,
    generation: VISUAL_RUNTIME_GENERATION,
    component: {
      installationId: binding?.installationId ?? "33333333-3333-4333-8333-333333333333",
      digest: binding?.digest ?? VISUAL_PLUGIN_DIGEST,
      componentId,
    },
    binding,
    compose: () => {
      throw new Error("The visual fixture has no component composition");
    },
  };
}

import { type HookTrustMutationOwner } from "@/features/hooks/application/hookTrust";
import { registerHookDataProviders } from "@/features/hooks/adapters/runtimeDataProviders";
import { type FlameClient } from "@flame/runtime-client";
import { lazy } from "react";
import { definePlugin } from "@/extensions/definePlugin";
import { registerSettingsPane } from "@/features/settings/public/registerSettingsPane";
import { HOOKS_PANE } from "@/features/settings/public/paneIDs";
import { createRuntimeHookTrustGateway } from "@/features/hooks/adapters/runtimeHookTrustGateway";
import { RUNTIME_STREAM, followRuntimeGeneration } from "@/features/connection/public/services";
const HooksPane = lazy(() =>
  import("@/features/hooks/ui/HooksPane").then(({ HooksPane }) => ({ default: HooksPane })),
);

export function createHooksPlugin(runtimeClient: () => FlameClient, trust: HookTrustMutationOwner) {
  return definePlugin({
    name: "flame.builtin.hooks-pane",
    requires: { runtime: RUNTIME_STREAM },
    setup(ctx) {
      registerHookDataProviders(ctx, runtimeClient);
      ctx.cleanup(() => trust.retireRuntimeGeneration());
      const replace = () =>
        trust.replaceRuntimeGeneration(() => createRuntimeHookTrustGateway(runtimeClient()));
      if (ctx.runtime.connectionGeneration() !== null) replace();
      ctx.cleanup(
        followRuntimeGeneration(ctx.runtime, (next) => {
          if (next === null) trust.retireRuntimeGeneration();
          else replace();
        }),
      );
      registerSettingsPane(ctx, {
        id: HOOKS_PANE,
        label: "settings.pane.hooks",
        keywords: [
          "hooks.trust",
          "hooks.trust.sub",
          "hooks.matcher",
          "hooks.kind.inject",
          "hooks.scope.global",
          "hooks.scope.project",
          "hooks.empty.sub",
        ],
        group: "agent",
        icon: "lightning",
        order: 57,
        component: HooksPane,
      });
    },
  });
}

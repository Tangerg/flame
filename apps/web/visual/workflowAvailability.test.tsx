import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { expect, it, vi } from "vitest";
import type { FlameClient } from "@flame/runtime-client";
import { createTestClient } from "@/test/renderClient";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";
import { definePlugin } from "@/extensions/definePlugin";
import { RUNTIME_STREAM, RuntimeConnectionGeneration } from "@/features/connection/public/services";
import { createInstalledPluginLoader } from "@/features/plugins/index";
import { createGoalPlugin } from "@/features/goal/index";
import { submitComposer } from "@/features/composer/application/submitComposer";
import { Composer } from "@/features/composer/ui/Composer";
import goalContract from "../../../plugins/goals/package/io.github.tangerg.flame/contracts/capability.generated.json";

it("withdraws the Goal suggestion on a backend change without discarding the typed draft", async () => {
  const source = {
    installationId: "goal-installation",
    contribution: "goal-workflow",
    role: goalContract.reference,
  };
  let disabled = false;
  let changed = () => {};
  let reads = 0;
  const client = {
    plugins: {
      list: async () => ({
        data: [
          {
            id: source.installationId,
            enabled: true,
            disabledServers: disabled ? ["goals"] : [],
            presentation: "admitted",
            realization: { type: "available" },
            selected: {
              digest: "f".repeat(64),
              components: [],
              locales: [],
              themes: [],
              name: "goals",
              contributions: [
                { id: source.contribution, server: "goals", selectionRoles: [source.role] },
              ],
            },
          },
        ],
      }),
      listProviders: async () => {
        reads++;
        return { data: [source] };
      },
    },
    runtimeEvents: {
      subscribe: async (_: unknown, signal: AbortSignal) => ({
        events: new ReadableStream({
          start(controller) {
            changed = () => controller.enqueue({});
            signal.addEventListener("abort", () => controller.close(), { once: true });
          },
        }),
      }),
    },
  } as unknown as FlameClient;
  const fixture = createTestClient({ runtimeClient: () => client });
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const generation = RuntimeConnectionGeneration.forProcess("workflow-ui");
  function Draft() {
    const [value, onChange] = useState("");
    return (
      <Composer
        value={value}
        onChange={onChange}
        onSend={() => true}
        images={[]}
        pastes={[]}
        acceptsImages
        onRemoveImage={() => {}}
        onAddImages={() => {}}
        onRemovePaste={() => {}}
        onEditPaste={() => true}
        onRestorePaste={() => {}}
        onAddPaste={() => {}}
        captureSubmission={() => ({
          value,
          images: [],
          attachmentsReady: true,
          pastes: [],
          consume: () => true,
        })}
      />
    );
  }
  try {
    await loadPluginsForTest(
      definePlugin({
        name: "test.workflow-stream",
        provides: { stream: RUNTIME_STREAM },
        setup: () => ({
          stream: {
            connectionGeneration: () => generation,
            subscribeConnection: () => () => {},
            reportConnectionLoss: () => {},
          },
        }),
      }),
      createInstalledPluginLoader(fixture.installedPlugins),
      createGoalPlugin(
        fixture.agent.access,
        fixture.composer.access,
        fixture.runtime,
        () => client,
        fixture.goals,
        fixture.installedPlugins.access.workflows,
      ),
    );
    fixture.render(
      <QueryClientProvider client={cache}>
        <Draft />
      </QueryClientProvider>,
    );
    const input = screen.getByRole("textbox") as HTMLTextAreaElement;
    fireEvent.change(input, { target: { value: "/go", selectionStart: 3 } });
    await screen.findByRole("option", { name: /\/goal/ });
    const previous = reads;
    act(() => {
      disabled = true;
      changed();
    });
    await waitFor(() => expect(reads).toBeGreaterThan(previous));
    await waitFor(() => expect(screen.queryByRole("option", { name: /\/goal/ })).toBeNull());
    expect(input.value).toBe("/go");
    const sendInput = vi.fn(() => true);
    const consume = vi.fn(() => true);
    submitComposer({
      draft: { value: "/goal release", images: [], pastes: [], attachmentsReady: true, consume },
      canSend: () => true,
      restoreFocus: () => {},
      sendInput,
    });
    expect(sendInput).not.toHaveBeenCalled();
    expect(consume).not.toHaveBeenCalled();
    act(() => {
      disabled = false;
      changed();
    });
    await screen.findByRole("option", { name: /\/goal/ });
  } finally {
    cleanup();
    await resetKernelForTest();
    await fixture.dispose();
    cache.clear();
  }
});

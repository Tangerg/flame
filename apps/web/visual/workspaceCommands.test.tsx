import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { createFlameClient, type FlameClient } from "@flame/runtime-client";
import { CodeHighlightProvider, FontScaleProvider, TooltipProvider } from "@flame/ui";
import { afterEach, beforeEach, expect, it } from "vitest";
import { executeCommand } from "@/features/command/public/commands";
import { HighlightedCodeSource } from "@/lib/highlight/HighlightedCodeSource";
import { queryClient } from "@/lib/queryClient";
import { resetKernelForTest } from "@/test/kernel";
import { createVisualClient, type VisualClient } from "./client";
import { createVisualRuntimeTransport } from "./createVisualRuntimeTransport";
import {
  installVisualWorkspaceFixture,
  visualWorkspaceRuntime,
} from "./installVisualWorkspaceFixture";
import { VisualWorkspaceFixture } from "./VisualWorkspaceFixture";

let visual: VisualClient;
let client: FlameClient;

beforeEach(async () => {
  await import("@/features/chat/message/ui/markdown/MarkdownRenderer");
  const runtime = visualWorkspaceRuntime("dock-light");
  client = createFlameClient(
    createVisualRuntimeTransport(runtime.snapshot, runtime.capabilities, runtime.readDiff),
    { capabilities: () => runtime.capabilities },
  );
  visual = createVisualClient(() => client);
  await installVisualWorkspaceFixture(visual, () => client, "dock-light", "light", runtime);
  render(
    <visual.Wrapper>
      <QueryClientProvider client={queryClient}>
        <FontScaleProvider fontSize={14}>
          <CodeHighlightProvider component={HighlightedCodeSource}>
            <TooltipProvider>
              <VisualWorkspaceFixture state="dock-light" />
            </TooltipProvider>
          </CodeHighlightProvider>
        </FontScaleProvider>
      </QueryClientProvider>
    </visual.Wrapper>,
  );
});

afterEach(async () => {
  cleanup();
  await resetKernelForTest();
  await visual.dispose();
  client.close();
  queryClient.clear();
});

it.each(["sidebar", "settings"])(
  "returns from %s through the installed Composer command",
  async (source) => {
    const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message composer" });
    fireEvent.change(draft, { target: { value: "Keep this draft — 保留" } });
    act(() => {
      draft.focus();
      draft.setSelectionRange(5, 5);
    });
    if (source === "settings") {
      await act(() => executeCommand("settings.open"));
      act(() => screen.getByRole("searchbox").focus());
    } else {
      act(() =>
        screen.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).focus(),
      );
    }

    await act(() => executeCommand("composer.focus"));

    expect(document.activeElement).toBe(draft);
    expect(draft.value).toBe("Keep this draft — 保留");
    expect(draft.selectionStart).toBe(5);
    expect(draft.selectionEnd).toBe(5);
    expect(visual.navigation.get().view).toBeNull();
  },
);

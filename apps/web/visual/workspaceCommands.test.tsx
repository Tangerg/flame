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

function pressEscape(target: Element) {
  fireEvent(
    target,
    new KeyboardEvent("keydown", {
      key: "Escape",
      code: "Escape",
      bubbles: true,
      cancelable: true,
    }),
  );
}

it.each(["material", "settings"])(
  "returns from %s through the installed Escape binding",
  async (source) => {
    const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message composer" });
    fireEvent.change(draft, { target: { value: "Keep this draft — 保留" } });
    act(() => {
      draft.focus();
      draft.setSelectionRange(5, 5);
    });
    let target: HTMLElement;
    if (source === "settings") {
      await act(() => executeCommand("settings.open"));
      target = screen.getByRole("tab", { name: "Appearance" });
    } else {
      fireEvent.click(screen.getByRole("button", { name: /^Open material full width/ }));
      target = screen.getByRole("region", { name: "Files" });
    }
    expect(visual.navigation.get().view).toBe(source === "settings" ? "settings" : "file");
    act(() => target.focus());

    pressEscape(target);
    expect(visual.navigation.get().view).toBeNull();

    await act(() => executeCommand("composer.focus"));
    expect(document.activeElement).toBe(draft);
    expect(draft.value).toBe("Keep this draft — 保留");
    expect(draft.selectionStart).toBe(5);
    expect(draft.selectionEnd).toBe(5);
  },
);

it("leaves Escape in the focused Settings search to that field", async () => {
  await act(() => executeCommand("settings.open"));
  const search = screen.getByRole("searchbox");
  act(() => search.focus());

  pressEscape(search);

  expect(visual.navigation.get().view).toBe("settings");
  expect(document.activeElement).toBe(search);
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

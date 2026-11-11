import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { createFlameClient, type FlameClient } from "@flame/runtime-client";
import { CodeHighlightProvider, FontScaleProvider, TooltipProvider } from "@flame/ui";
import { afterEach, beforeEach, expect, it, onTestFinished } from "vitest";
import { executeCommand } from "@/features/command/public/commands";
import commandMenu from "@/features/command/menu/index";
import { HighlightedCodeSource } from "@/lib/highlight/HighlightedCodeSource";
import { queryClient } from "@/lib/queryClient";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";
import { createVisualClient, type VisualClient } from "./client";
import { createVisualRuntimeTransport } from "./createVisualRuntimeTransport";
import {
  installVisualWorkspaceFixture,
  visualWorkspaceRuntime,
} from "./installVisualWorkspaceFixture";
import { VisualWorkspaceFixture } from "./VisualWorkspaceFixture";
import { activeLocale, setLocale, t } from "@/lib/i18n/index";
import { presentationTitle } from "@/lib/i18n/presentationTitle";
import trajectoryDeclaration from "../../../../plugins/trajectory/package/io.github.tangerg.flame/declaration.json";
import brandDeclaration from "../../../../plugins/brand/package/io.github.tangerg.flame/declaration.json";
import providersDeclaration from "../../../../plugins/providers/package/io.github.tangerg.flame/declaration.json";
import mcpDeclaration from "../../../../plugins/mcp/package/io.github.tangerg.flame/declaration.json";
import hooksDeclaration from "../../../../plugins/hooks/package/io.github.tangerg.flame/declaration.json";

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

it("reveals a command-menu material in the narrow workspace and returns to its source", async () => {
  await act(() => loadPluginsForTest(commandMenu));
  const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message composer" });
  fireEvent.change(draft, { target: { value: "Keep this command source — 保留" } });
  act(() => {
    draft.focus();
    draft.setSelectionRange(5, 5);
  });
  await act(() => executeCommand("command.menu"));
  const query = await screen.findByRole("combobox", { name: "Run a command…" });
  fireEvent.change(query, { target: { value: "Workspace diff" } });

  await act(async () => fireEvent.keyDown(query, { key: "Enter" }));

  const material = await screen.findAllByRole("region", { name: "Workspace diff" });
  await waitFor(() =>
    expect(material.some((region) => region.contains(document.activeElement))).toBe(true),
  );
  expect(draft.value).toBe("Keep this command source — 保留");
  const back = screen.getByRole("button", { name: "Back to conversation" });
  fireEvent.click(back);

  await waitFor(() => expect(document.activeElement).toBe(draft));
  expect(draft.selectionStart).toBe(5);
  expect(draft.selectionEnd).toBe(5);
  expect(draft.value).toBe("Keep this command source — 保留");
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

it.each(["zh", "de"])(
  "keeps declared contribution names and the same draft through %s navigation",
  async (locale) => {
    const originalLocale = activeLocale();
    onTestFinished(() => setLocale(originalLocale));
    const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message composer" });
    const value = "  Keep the authored titles and this draft. 保留原文。  ";
    fireEvent.change(draft, { target: { value } });
    act(() => {
      draft.focus();
      draft.setSelectionRange(7, 7);
    });
    try {
      await act(() => setLocale(locale));
      await act(() => loadPluginsForTest(commandMenu));
      await act(() => executeCommand("command.menu"));
      const titleFor = (entry: {
        title: string;
        localizedTitles?: Readonly<Record<string, string>>;
      }) => presentationTitle(entry.title, entry.localizedTitles, locale);
      const trajectory = trajectoryDeclaration.components.find(
        (entry) => entry.id === "trajectory",
      )!;
      const title = titleFor(trajectory);
      const query = await screen.findByRole("combobox");
      fireEvent.change(query, { target: { value: title } });
      expect(
        await screen.findByRole("option", {
          name: t("commandMenu.view", { title }),
        }),
      ).toBeTruthy();
      await act(async () => fireEvent.keyDown(query, { key: "Enter" }));
      await screen.findAllByRole("region", { name: title });

      await act(() => executeCommand("settings.open"));
      for (const [declaration, id] of [
        [brandDeclaration, "brand"],
        [providersDeclaration, "settings"],
        [mcpDeclaration, "settings"],
        [hooksDeclaration, "settings"],
      ] as const) {
        const entry = declaration.components.find((component) => component.id === id)!;
        expect(screen.getByRole("tab", { name: titleFor(entry) })).toBeTruthy();
      }
      await act(() => executeCommand("composer.focus"));
      expect(document.activeElement).toBe(draft);
      expect(draft.value).toBe(value);
      expect([draft.selectionStart, draft.selectionEnd]).toEqual([7, 7]);
    } finally {
      if (screen.queryByRole("combobox", { name: t("commandMenu.placeholder") }))
        await act(() => executeCommand("command.menu"));
    }
  },
);

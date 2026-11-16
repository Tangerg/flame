import { WindowNotifications } from "@/features/notifications/toaster/window";
import type { ReactNode } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { isCancelledError, QueryClientProvider, QueryObserver } from "@tanstack/react-query";
import { createFlameClient, type FlameClient } from "@flame/runtime-client";
import { CodeHighlightProvider, FontScaleProvider, TooltipProvider } from "@flame/ui";
import { afterEach, beforeEach, expect, it, onTestFinished, vi } from "vitest";
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
import {
  WORKSPACE_LIST_FILES_KEY,
  workspaceFileListQuery,
} from "@/features/workspace/application/workspaceQueries";
import { lookupDataProvider } from "@/extensions/dataProvider";
import { VISUAL_WORKSPACE_PATH } from "./agentFixtureFacts";
import {
  VISUAL_DIRECTORY_PATH,
  VISUAL_DIRECTORY_CHANGED_EVENT,
  type VisualDirectoryState,
} from "./workspaceFixtureStates";
import { type RuntimeSubscribeRequest } from "@flame/runtime-client/wire";

let visual: VisualClient;
let client: FlameClient;
let transport: ReturnType<typeof createVisualRuntimeTransport>;
let renderContent: (node: ReactNode) => void;

async function installFixture(directoryState?: VisualDirectoryState, renderView = true) {
  await import("@/features/chat/message/ui/markdown/MarkdownRenderer");
  const runtime = visualWorkspaceRuntime("dock-light");
  transport = createVisualRuntimeTransport(
    runtime.snapshot,
    runtime.capabilities,
    runtime.readDiff,
  );
  const ownedClient = createFlameClient(transport, { capabilities: () => runtime.capabilities });
  client = ownedClient;
  render(
    <WindowNotifications
      start={(messages, update) => {
        visual = createVisualClient(() => ownedClient, messages);
        renderContent = update;
        return () => {};
      }}
    />,
  );
  await installVisualWorkspaceFixture(visual, () => ownedClient, "dock-light", "light", runtime, {
    sidebarOpen: true,
    directoryState,
  });
  if (!renderView) return;
  act(() =>
    renderContent(
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
    ),
  );
}

beforeEach(() => installFixture());

async function retireFixture() {
  cleanup();
  await resetKernelForTest();
  await visual.dispose();
  await client.close();
}

afterEach(async () => {
  await retireFixture();
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

async function installDirectoryState(directoryState: VisualDirectoryState) {
  await retireFixture();
  await installFixture(directoryState, false);
}

function directoryQueryOptions() {
  return {
    ...workspaceFileListQuery({ cwd: VISUAL_WORKSPACE_PATH, path: VISUAL_DIRECTORY_PATH }),
    staleTime: 0,
    retryDelay: 0,
  };
}

it("keeps directory failure through automatic retries and resets it on fixture reinstallation", async () => {
  await installDirectoryState("error");
  const query = directoryQueryOptions();
  queryClient.setQueryData(query.queryKey, [
    { path: "apps/previous.ts", name: "previous.ts", type: "file" },
  ]);

  await expect(queryClient.fetchQuery(query)).rejects.toThrow("could not list apps");
  expect(queryClient.getQueryState(query.queryKey)).toMatchObject({
    status: "error",
    fetchStatus: "idle",
    fetchFailureCount: 2,
  });
  const accepted = await queryClient.fetchQuery(query);
  expect(accepted).toHaveLength(1);
  expect(accepted[0]?.path).not.toBe("apps/previous.ts");

  await installDirectoryState("error");
  await expect(queryClient.fetchQuery(query)).rejects.toThrow("could not list apps");
  await expect(queryClient.fetchQuery(query)).resolves.toEqual(accepted);
});

it("retains accepted directory children through refresh failure and ignores manual cache success", async () => {
  await installDirectoryState("refresh-error");
  const query = directoryQueryOptions();
  const accepted = await queryClient.fetchQuery(query);
  await expect(queryClient.fetchQuery(query)).resolves.toEqual(accepted);
  window.dispatchEvent(new Event(VISUAL_DIRECTORY_CHANGED_EVENT));
  await expect(queryClient.fetchQuery(query)).rejects.toThrow("could not list apps");
  expect(queryClient.getQueryData(query.queryKey)).toBe(accepted);

  queryClient.setQueryData(query.queryKey, accepted);
  await expect(queryClient.fetchQuery(query)).resolves.toEqual(accepted);
});

it("routes a notified directory change through the installed Workspace event consumer", async () => {
  await installDirectoryState("refresh-error");
  const subscriptions: Array<{ request: RuntimeSubscribeRequest; signal?: AbortSignal }> = [];
  const subscribe = client.runtimeEvents.subscribe.bind(client.runtimeEvents);
  const subscriptionWitness = vi
    .spyOn(client.runtimeEvents, "subscribe")
    .mockImplementation(async (request, signal) => {
      const opened = await subscribe(request, signal);
      subscriptions.push({ request, signal });
      return opened;
    });
  const notify = () => transport.notifyDirectoryChange();
  window.addEventListener(VISUAL_DIRECTORY_CHANGED_EVENT, notify);
  let successfulReads = 0;
  const stopWitness = queryClient.getQueryCache().subscribe((event) => {
    const request = event.query.queryKey[1] as { cwd?: string; path?: string } | undefined;
    if (
      event.type === "updated" &&
      event.query.queryKey[0] === WORKSPACE_LIST_FILES_KEY &&
      request?.cwd === VISUAL_WORKSPACE_PATH &&
      request.path === VISUAL_DIRECTORY_PATH &&
      event.action.type === "success" &&
      !event.action.manual
    )
      successfulReads += 1;
  });
  onTestFinished(() => {
    subscriptionWitness.mockRestore();
    window.removeEventListener(VISUAL_DIRECTORY_CHANGED_EVENT, notify);
    stopWitness();
  });
  const observer = new QueryObserver(queryClient, directoryQueryOptions());
  const stop = observer.subscribe(() => {});
  try {
    await waitFor(() => {
      expect(successfulReads).toBeGreaterThan(1);
      expect(observer.getCurrentResult()).toMatchObject({ status: "success", fetchStatus: "idle" });
      expect(
        subscriptions.some(
          ({ request, signal }) =>
            !signal?.aborted &&
            request.topics.includes("files.changed") &&
            request.watches?.some(
              (watch) =>
                watch.workspace.path === VISUAL_WORKSPACE_PATH &&
                watch.paths?.includes(VISUAL_DIRECTORY_PATH),
            ),
        ),
      ).toBe(true);
    });
    const accepted = observer.getCurrentResult().data;
    expect(accepted).toHaveLength(1);

    window.dispatchEvent(new Event(VISUAL_DIRECTORY_CHANGED_EVENT));
    await waitFor(() => expect(observer.getCurrentResult().status).toBe("error"));
    expect(observer.getCurrentResult().data).toBe(accepted);
    await observer.refetch();
    expect(observer.getCurrentResult()).toMatchObject({ status: "success", fetchStatus: "idle" });
    expect(observer.getCurrentResult().data).toEqual(accepted);
  } finally {
    stop();
    observer.destroy();
  }
});

it("aborts a pending directory read when its fixture retires", async () => {
  await installDirectoryState("loading");
  const query = directoryQueryOptions();
  const controller = new AbortController();
  const cause = new Error("Directory read retired");
  const fetcher = lookupDataProvider(WORKSPACE_LIST_FILES_KEY)!;
  const providerOutcome = fetcher(
    { cwd: VISUAL_WORKSPACE_PATH, path: VISUAL_DIRECTORY_PATH },
    controller.signal,
  ).catch((error: unknown) => error);
  controller.abort(cause);
  expect(await providerOutcome).toBe(cause);

  const outcome = queryClient.fetchQuery(query).catch((error: unknown) => error);
  await waitFor(() =>
    expect(queryClient.getQueryState(query.queryKey)?.fetchStatus).toBe("fetching"),
  );

  await resetKernelForTest();
  expect(isCancelledError(await outcome)).toBe(true);
  expect(queryClient.getQueryState(query.queryKey)?.fetchStatus).not.toBe("fetching");

  await installDirectoryState("error");
  await expect(queryClient.fetchQuery(query)).rejects.toThrow("could not list apps");
});

it("limits an empty directory scene to its captured nonrecursive workspace request", async () => {
  await installDirectoryState("empty");
  await expect(queryClient.fetchQuery(directoryQueryOptions())).resolves.toEqual([]);
  const root = await queryClient.fetchQuery(workspaceFileListQuery({ cwd: VISUAL_WORKSPACE_PATH }));
  expect(root.map(({ path }) => path)).toEqual(
    expect.arrayContaining([VISUAL_DIRECTORY_PATH, "README.md"]),
  );
  for (const request of [
    { cwd: "/visual/another-workspace", path: VISUAL_DIRECTORY_PATH },
    { cwd: VISUAL_WORKSPACE_PATH, path: VISUAL_DIRECTORY_PATH, recursive: true },
  ]) {
    const children = await queryClient.fetchQuery(workspaceFileListQuery(request));
    expect(children).toHaveLength(1);
    expect(children[0]?.type).toBe("file");
  }
});

it.each([true, false])(
  "does not advance directory failure after cancellation (revert=%s)",
  async (revert) => {
    await installDirectoryState("error");
    const query = directoryQueryOptions();
    const previous = new Error("Previous directory read failed");
    await expect(
      queryClient.fetchQuery({
        ...query,
        retry: false,
        queryFn: async () => {
          throw previous;
        },
      }),
    ).rejects.toBe(previous);

    const outcome = queryClient.fetchQuery(query).catch((error: unknown) => error);
    await queryClient.cancelQueries({ queryKey: query.queryKey, exact: true }, { revert });
    expect(isCancelledError(await outcome)).toBe(true);

    await expect(queryClient.fetchQuery(query)).rejects.toThrow("could not list apps");
    await expect(queryClient.fetchQuery(query)).resolves.toHaveLength(1);
  },
);

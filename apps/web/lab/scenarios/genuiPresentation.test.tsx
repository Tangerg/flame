import { unavailableComponentNotifications } from "@/test/unavailableNotifications";
import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { ApplicationCalloutLabels } from "@/app/calloutLabels";
import { afterEach, beforeEach, expect, it, onTestFinished, vi } from "vitest";
import {
  createTranslator,
  type CapturedToolResult,
  type CapturedToolInteraction,
  type ComponentContext,
  type PluginComponent,
  type Translate,
} from "@flame/component-sdk";
import * as ui from "@flame/ui";
import { ComponentBoundary } from "@/features/plugins/ui/InstalledComponent";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { GenUIResult } from "../../../../plugins/genui/component/component";
import { MarkdownView, YouTubeView } from "../../../../plugins/genui/component/media";
import { en } from "../../../../plugins/genui/component/locales/en";
import { zh } from "../../../../plugins/genui/component/locales/zh";
import { zhTW } from "../../../../plugins/genui/component/locales/zh-TW";
import { ja } from "../../../../plugins/genui/component/locales/ja";
import { ko } from "../../../../plugins/genui/component/locales/ko";
import { es } from "../../../../plugins/genui/component/locales/es";
import { fr } from "../../../../plugins/genui/component/locales/fr";
import { de } from "../../../../plugins/genui/component/locales/de";
import {
  Presentation,
  type AcceptedDocument,
  type UiNode,
} from "../../../../plugins/genui/contracts/document";
import { GENUI_REPORT } from "./genuiReport";
import { preventBrowserNavigation } from "@/test/browserTasks";
import { limits } from "../../../../plugins/genui/contracts/expression";

it.each(Object.entries({ en, zh, "zh-TW": zhTW, ja, ko, es, fr, de }))(
  "keeps duplicate Markdown footnotes local and labeled in %s",
  (locale, catalog) => {
    const t = createTranslator({ en, [locale]: catalog }, locale);
    const text = "Claim[^来源]. Again[^来源].\n\n[^来源]: Supporting note.";
    const scene = () => <MarkdownView props={{ text }} renderNode={() => null} t={t} />;
    const view = testClient.render(
      <>
        {scene()}
        {scene()}
      </>,
    );
    const sections = view.container.querySelectorAll<HTMLElement>("[data-footnotes]");
    expect(sections).toHaveLength(2);
    const identities = [...view.container.querySelectorAll("[id]")].map((node) => node.id);
    expect(new Set(identities).size).toBe(identities.length);
    for (const section of sections) {
      const root = section.parentElement!;
      const label = section.querySelector<HTMLElement>(".sr-only")!;
      expect(label.textContent).toBe(t("markdown.footnotes"));
      const definition = section.querySelector<HTMLElement>("li[id]")!;
      const references = root.querySelectorAll<HTMLAnchorElement>("[data-footnote-ref]");
      const backlinks = section.querySelectorAll<HTMLAnchorElement>("[data-footnote-backref]");
      expect(references).toHaveLength(2);
      expect(backlinks).toHaveLength(2);
      for (const [index, reference] of references.entries()) {
        expect(reference.getAttribute("href")).toBe(`#${definition.id}`);
        expect(reference.getAttribute("aria-describedby")).toBe(label.id);
        expect(reference.ownerDocument.getElementById(reference.id)).toBe(reference);
        expect(reference.getAttribute("target")).toBeNull();
        const backlink = backlinks[index]!;
        expect(backlink.getAttribute("href")).toBe(`#${reference.id}`);
        expect(backlink.getAttribute("target")).toBeNull();
        expect(backlink.getAttribute("aria-label")).toBe(
          t("markdown.backReference", {
            reference: index === 0 ? "1" : "1-2",
          }),
        );
      }
    }
    view.rerender(
      <>
        {scene()}
        {scene()}
      </>,
    );
    expect([...view.container.querySelectorAll("[id]")].map((node) => node.id)).toEqual(identities);
  },
);

it("renders named captured tasks without a form checkbox or an extra keyboard stop", () => {
  const text =
    "- [x] Accepted **decision**\n- [ ] Pending item\n\n  Supporting paragraph.\n\n  - Nested ordinary item\n- Ordinary item";
  const view = testClient.render(
    <MarkdownView props={{ text }} renderNode={() => null} t={createTranslator({ en }, "en")} />,
  );
  const checked = screen.getByRole("checkbox", { name: "Accepted decision" });
  const pending = screen.getByRole("checkbox", { name: "Pending item" });
  expect(checked.getAttribute("aria-checked")).toBe("true");
  expect(pending.getAttribute("aria-checked")).toBe("false");
  for (const marker of [checked, pending]) {
    expect(marker.getAttribute("aria-disabled")).toBe("true");
    expect(marker.hasAttribute("tabindex")).toBe(false);
  }
  expect(view.container.querySelector('input[type="checkbox"]')).toBeNull();
  expect(view.container.textContent).toContain("Supporting paragraph.");
  expect(view.container.textContent).toContain("Nested ordinary item");
});

it("keeps fragment navigation local without broadening GenUI external URL admission", () => {
  testClient.render(
    <MarkdownView
      props={{
        text: "[Local](#local) [Web](https://example.test) [Denied](javascript:alert%281%29)",
      }}
      renderNode={() => null}
      t={createTranslator({ en }, "en")}
    />,
  );
  expect(screen.getByRole("link", { name: "Local" }).getAttribute("target")).toBeNull();
  expect(screen.getByRole("link", { name: "Web" }).getAttribute("target")).toBe("_blank");
  expect(screen.queryByRole("link", { name: "Denied" })).toBeNull();
});

let testClient: TestClient;
beforeEach(() => {
  const owned = createTestClient({ start: false });
  testClient = owned;
  onTestFinished(() => owned.dispose());
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

function capturedContext(
  result: string,
  locale = "en",
  recorded: Partial<CapturedToolResult> = {},
  options: { signal?: AbortSignal; interaction?: CapturedToolInteraction } = {},
): ComponentContext {
  return Object.freeze({
    component: Object.freeze({
      installationId: "captured-genui",
      digest: "a".repeat(64),
      componentId: "genui-result",
    }),
    locale,
    signal: options.signal ?? new AbortController().signal,
    ...(options.interaction ? { interaction: options.interaction } : {}),
    toolResult: Object.freeze({
      id: "item_genui_result",
      runId: "run_genui_result",
      name: "genui_ui_present",
      fn: "genui_ui_present",
      args: "{}",
      status: "ok" as const,
      result,
      ...recorded,
    }),
    get runtime(): never {
      throw new Error("local genui must not request live data");
    },
    get telemetry(): never {
      throw new Error("local genui must not request telemetry");
    },
    get sessionId(): never {
      throw new Error("captured genui must not follow current navigation");
    },
    track(): never {
      throw new Error("local genui must not start i/o");
    },
    saveFile(): never {
      throw new Error("local genui must not save files");
    },
    notifications: unavailableComponentNotifications,
    subscribeRootRunSettlements() {
      throw new Error("This fixture must not observe run settlements");
    },
    registerCommand() {
      throw new Error("This fixture must not register client commands");
    },
    compose(): never {
      throw new Error("genui must not compose another component");
    },
  });
}

function document(
  nodes: UiNode[],
  state: AcceptedDocument["state"] = [],
  actions: AcceptedDocument["actions"] = [],
): AcceptedDocument {
  return { title: "Interactive review", state, nodes, actions, operations: [], contributions: [] };
}

function show(value: AcceptedDocument, locale = "en") {
  const context = capturedContext(JSON.stringify(value), locale);
  return { context, ...testClient.render(<GenUIResult context={context} />) };
}

function displayContext(context: ComponentContext, locale: string): ComponentContext {
  const compose: ComponentContext["compose"] = (composition) => context.compose(composition);
  return Object.freeze(
    Object.defineProperties(
      {},
      {
        ...Object.getOwnPropertyDescriptors(context),
        locale: { value: locale, enumerable: true },
        compose: { value: compose, enumerable: true },
      },
    ),
  ) as ComponentContext;
}

const followupReport = document(
  [
    { id: "note", type: "Input", props: { field: "note", label: "Review note" } },
    { id: "send", type: "Button", props: { label: "Send readout", action: "send" } },
    { id: "delivery", type: "Text", props: { text: { state: "delivery" } } },
  ],
  [
    { id: "note", value: "Captured note" },
    { id: "delivery", value: "No delivery accepted." },
  ],
  [
    {
      id: "send",
      steps: [
        {
          type: "followup",
          message: { literal: "Review captured note" },
          payload: { object: { note: { state: "note" } } },
        },
        { type: "set", state: "delivery", value: { literal: "Original followup accepted." } },
      ],
    },
  ],
);

function followupScene(context: ComponentContext) {
  return (
    <>
      <GenUIResult context={context} />
      <input aria-label="Later draft" defaultValue="Continue authoring" />
    </>
  );
}

it("retains the original followup recovery and blocked action through display locale changes", async () => {
  const owner = new AbortController();
  onTestFinished(() => owner.abort());
  const recover = vi.fn(async () => ({
    status: "accepted" as const,
    sessionId: "ses_original",
    runId: "run_original",
    userItemId: "item_input",
  }));
  const submit = vi.fn<CapturedToolInteraction["submit"]>(async () => ({
    status: "unknown",
    idempotencyKey: "original-followup",
    recover,
  }));
  const context = capturedContext(
    JSON.stringify(followupReport),
    "en",
    {},
    {
      signal: owner.signal,
      interaction: { source: { runId: "run_genui_result", itemId: "item_genui_result" }, submit },
    },
  );
  const view = testClient.render(followupScene(context));
  const note = screen.getByRole<HTMLInputElement>("textbox", { name: "Review note" });
  const send = screen.getByRole<HTMLButtonElement>("button", { name: "Send readout" });
  await waitFor(() => expect(send.disabled).toBe(false));
  fireEvent.change(note, { target: { value: "Original review 保留" } });
  await act(async () => fireEvent.click(send));
  const recovery = await screen.findByRole("button", { name: "Recover original action" });
  expect(screen.getByText("send · original-followup")).toBeTruthy();
  expect(send.disabled).toBe(true);
  const later = screen.getByRole<HTMLInputElement>("textbox", { name: "Later draft" });
  act(() => later.focus());
  fireEvent.change(later, { target: { value: "Continue the later task" } });
  const localized = displayContext(context, "zh");

  await act(async () => view.rerender(followupScene(localized)));

  expect(owner.signal.aborted).toBe(false);
  expect(localized.toolResult).toBe(context.toolResult);
  expect(localized.interaction).toBe(context.interaction);
  expect(screen.getByRole("button", { name: "恢复原动作" })).toBe(recovery);
  expect(screen.getByText("send · original-followup")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Send readout" })).toBe(send);
  expect(screen.getByRole("textbox", { name: "Review note" })).toBe(note);
  expect(note.value).toBe("Original review 保留");
  expect(note.readOnly).toBe(true);
  expect(send.disabled).toBe(true);
  expect(window.document.activeElement).toBe(later);
  fireEvent.click(send);
  expect(submit).toHaveBeenCalledOnce();
  expect(submit).toHaveBeenCalledWith({
    action: "Review captured note",
    payloadJSON: '{"note":"Original review 保留"}',
  });
  await act(async () => fireEvent.click(recovery));
  expect(await screen.findByText("动作已接受。")).toBeTruthy();
  expect(screen.getByText("Original followup accepted.")).toBeTruthy();
  expect(recover).toHaveBeenCalledOnce();
  expect(submit).toHaveBeenCalledOnce();
  expect(send.disabled).toBe(false);
  expect(note.readOnly).toBe(false);
  expect(screen.queryByRole("button", { name: "恢复原动作" })).toBeNull();
  expect(window.document.activeElement).toBe(later);
  expect(later.value).toBe("Continue the later task");
  expect(context.toolResult!.result).toBe(JSON.stringify(followupReport));
});

it("keeps a held original followup and its editing state through a display locale change", async () => {
  const owner = new AbortController();
  onTestFinished(() => owner.abort());
  const reply = Promise.withResolvers<Awaited<ReturnType<CapturedToolInteraction["submit"]>>>();
  const submit = vi.fn<CapturedToolInteraction["submit"]>(() => reply.promise);
  const context = capturedContext(
    JSON.stringify(followupReport),
    "en",
    {},
    {
      signal: owner.signal,
      interaction: { source: { runId: "run_genui_result", itemId: "item_genui_result" }, submit },
    },
  );
  const view = testClient.render(followupScene(context));
  const note = screen.getByRole<HTMLInputElement>("textbox", { name: "Review note" });
  const send = screen.getByRole<HTMLButtonElement>("button", { name: "Send readout" });
  await waitFor(() => expect(send.disabled).toBe(false));
  fireEvent.change(note, { target: { value: "Held original draft 保留" } });
  try {
    await act(async () => fireEvent.click(send));
    expect(submit).toHaveBeenCalledOnce();
    expect(screen.getByText("Waiting for the operation…")).toBeTruthy();
    const later = screen.getByRole<HTMLInputElement>("textbox", { name: "Later draft" });
    act(() => later.focus());
    fireEvent.change(later, { target: { value: "Writing while the original waits" } });

    await act(async () => view.rerender(followupScene(displayContext(context, "zh"))));

    expect(owner.signal.aborted).toBe(false);
    expect(screen.getByRole("textbox", { name: "Review note" })).toBe(note);
    expect(screen.getByRole("button", { name: "Send readout" })).toBe(send);
    expect(note.value).toBe("Held original draft 保留");
    expect(note.readOnly).toBe(true);
    expect(send.disabled).toBe(true);
    expect(screen.getByText("正在等待操作结果…")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "恢复原动作" })).toBeNull();
    expect(window.document.activeElement).toBe(later);
    fireEvent.click(send);
    expect(submit).toHaveBeenCalledOnce();
  } finally {
    await act(async () =>
      reply.resolve({
        status: "accepted",
        sessionId: "ses_original",
        runId: "run_original",
        userItemId: "item_input",
      }),
    );
  }
  expect(await screen.findByText("动作已接受。")).toBeTruthy();
  expect(screen.getByText("Original followup accepted.")).toBeTruthy();
  expect(screen.getByRole("textbox", { name: "Review note" })).toBe(note);
  expect(note.value).toBe("Held original draft 保留");
  expect(note.readOnly).toBe(false);
  expect(send.disabled).toBe(false);
  expect(submit).toHaveBeenCalledOnce();
  expect(submit).toHaveBeenCalledWith({
    action: "Review captured note",
    payloadJSON: '{"note":"Held original draft 保留"}',
  });
  const later = screen.getByRole<HTMLInputElement>("textbox", { name: "Later draft" });
  expect(window.document.activeElement).toBe(later);
  expect(later.value).toBe("Writing while the original waits");
});

it("preserves permitted Markdown media and reading state through a full result locale update", async () => {
  const value = document([
    {
      id: "markdown",
      type: "Markdown",
      props: {
        text: '![Recorded diagram](https://example.test/diagram.png)\n\n```json\n{"source":"captured"}\n```\n\n| Source | Owner |\n| --- | --- |\n| captured | Runtime |',
      },
    },
  ]);
  const { context, rerender } = show(value);
  await act(async () => {});
  fireEvent.click(screen.getByRole("button", { name: "Load external content: Recorded diagram" }));
  const image = screen.getByRole("img", { name: "Recorded diagram" });
  fireEvent.load(image);
  const source = screen.getByRole("region", { name: "Code" });
  const table = screen.getByRole("region", { name: "Markdown table" });
  const trigger = screen.getByRole("button", { name: "View image: Recorded diagram" });
  act(() => trigger.focus());
  fireEvent.click(trigger);
  const preview = await screen.findByRole("dialog", { name: "Recorded diagram" });
  const zoom = within(preview).getByRole("button", { name: "Zoom in image" });
  act(() => zoom.focus());
  fireEvent.click(zoom);
  const viewport = within(preview).getByRole("region", { name: "Recorded diagram" });
  const chinese = createTranslator({ en, zh }, "zh");

  await act(async () => rerender(<GenUIResult context={displayContext(context, "zh")} />));

  expect(context.signal.aborted).toBe(false);
  expect(screen.getByRole("dialog", { name: "Recorded diagram" })).toBe(preview);
  expect(within(preview).getByRole("button", { name: chinese("image.zoomIn") })).toBe(zoom);
  expect(viewport.getAttribute("data-image-zoom")).toBe("125");
  expect(window.document.activeElement).toBe(zoom);
  fireEvent.keyDown(preview, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(window.document.activeElement).toBe(trigger));
  expect(screen.getByRole("button", { name: `${chinese("image.view")}: Recorded diagram` })).toBe(
    trigger,
  );
  expect(screen.getByRole("img", { name: "Recorded diagram" })).toBe(image);
  expect(
    screen.queryByRole("button", {
      name: chinese("resource.load", { resource: "Recorded diagram" }),
    }),
  ).toBeNull();
  expect(screen.getByRole("region", { name: chinese("code.label") })).toBe(source);
  expect(source.textContent).toBe('{"source":"captured"}\n');
  expect(screen.getByRole("region", { name: chinese("markdown.table") })).toBe(table);
});

it("resets the document on real signal retirement and fences a late original followup", async () => {
  const owner = new AbortController();
  const replacement = new AbortController();
  onTestFinished(() => {
    owner.abort();
    replacement.abort();
  });
  const reply = Promise.withResolvers<Awaited<ReturnType<CapturedToolInteraction["submit"]>>>();
  const submit = vi.fn<CapturedToolInteraction["submit"]>(() => reply.promise);
  const context = capturedContext(
    JSON.stringify(followupReport),
    "en",
    {},
    {
      signal: owner.signal,
      interaction: { source: { runId: "run_genui_result", itemId: "item_genui_result" }, submit },
    },
  );
  const view = testClient.render(followupScene(context));
  const oldNote = screen.getByRole<HTMLInputElement>("textbox", { name: "Review note" });
  const send = screen.getByRole<HTMLButtonElement>("button", { name: "Send readout" });
  await waitFor(() => expect(send.disabled).toBe(false));
  fireEvent.change(oldNote, { target: { value: "Retired submitted note" } });
  try {
    await act(async () => fireEvent.click(send));
    expect(submit).toHaveBeenCalledOnce();
    const next = Object.freeze(
      Object.defineProperties(
        {},
        {
          ...Object.getOwnPropertyDescriptors(displayContext(context, "zh")),
          signal: { value: replacement.signal, enumerable: true },
        },
      ),
    ) as ComponentContext;
    await act(async () => {
      owner.abort();
      view.rerender(followupScene(next));
    });
    const note = screen.getByRole<HTMLInputElement>("textbox", { name: "Review note" });
    await waitFor(() => expect(note.readOnly).toBe(false));
    expect(next.toolResult).toBe(context.toolResult);
    expect(owner.signal.aborted).toBe(true);
    expect(replacement.signal.aborted).toBe(false);
    expect(note).not.toBe(oldNote);
    expect(note.value).toBe("Captured note");
    expect(screen.getByText("No delivery accepted.")).toBeTruthy();
    fireEvent.change(note, { target: { value: "Continue under the new source" } });
    act(() => screen.getByRole("textbox", { name: "Later draft" }).focus());
  } finally {
    await act(async () =>
      reply.resolve({
        status: "accepted",
        sessionId: "ses_original",
        runId: "run_original",
        userItemId: "item_input",
      }),
    );
  }
  const note = screen.getByRole<HTMLInputElement>("textbox", { name: "Review note" });
  expect(note.value).toBe("Continue under the new source");
  expect(note.readOnly).toBe(false);
  expect(screen.getByText("No delivery accepted.")).toBeTruthy();
  expect(screen.queryByText("Original followup accepted.")).toBeNull();
  expect(screen.queryByText("动作已接受。")).toBeNull();
  expect(window.document.activeElement).toBe(screen.getByRole("textbox", { name: "Later draft" }));
  expect(submit).toHaveBeenCalledOnce();
  expect(submit).toHaveBeenCalledWith({
    action: "Review captured note",
    payloadJSON: '{"note":"Retired submitted note"}',
  });
});

it("delivers a local open action through document link navigation without retaining its anchor", async () => {
  onTestFinished(preventBrowserNavigation());
  const url = "https://www.openstreetmap.org/?mlat=51.5074&mlon=-0.1278";
  const value = document(
    [
      { id: "note", type: "Input", props: { field: "note", label: "Review note" } },
      { id: "open", type: "Button", props: { label: "Open captured source", action: "open" } },
    ],
    [{ id: "note", value: "Keep the captured context." }],
    [{ id: "open", steps: [{ type: "openUrl", url: { literal: url } }] }],
  );
  const destinations: Array<{ href: string; target: string; rel: string }> = [];
  let dispatched: HTMLAnchorElement | undefined;
  const readLink = (event: MouseEvent) => {
    if (!(event.target instanceof HTMLAnchorElement)) return;
    event.preventDefault();
    dispatched = event.target;
    destinations.push({ href: dispatched.href, target: dispatched.target, rel: dispatched.rel });
  };
  window.document.addEventListener("click", readLink);
  onTestFinished(() => window.document.removeEventListener("click", readLink));
  const { context } = show(value);
  const open = screen.getByRole<HTMLButtonElement>("button", { name: "Open captured source" });
  await waitFor(() => expect(open.disabled).toBe(false));
  act(() => open.focus());
  fireEvent.click(open);
  await waitFor(() =>
    expect(destinations).toEqual([{ href: url, target: "_blank", rel: "noopener noreferrer" }]),
  );
  expect(dispatched?.isConnected).toBe(false);
  expect(window.document.activeElement).toBe(open);
  expect(screen.getByRole("textbox", { name: "Review note" })).toHaveProperty(
    "value",
    "Keep the captured context.",
  );
  expect(context.toolResult!.result).toBe(JSON.stringify(value));
});

it("keeps original action recovery when a real Table search rejects an oversized value", async () => {
  const value = document(
    [
      {
        id: "inventory",
        type: "Table",
        props: {
          caption: "Inventory",
          rowKey: "id",
          columns: [{ key: "name", label: "Name" }],
          rows: [{ id: "one", name: "Memory" }],
          searchState: "search",
          pageState: "page",
          pageSize: 5,
        },
      },
      { id: "send", type: "Button", props: { label: "Send readout", action: "send" } },
    ],
    [
      { id: "search", value: "" },
      { id: "page", value: 0 },
    ],
    [
      {
        id: "send",
        steps: [
          {
            type: "followup",
            message: { literal: "Review inventory" },
            payload: { object: { search: { state: "search" } } },
          },
        ],
      },
    ],
  );
  const base = capturedContext(JSON.stringify(value));
  const recover = vi.fn(async () => ({
    status: "accepted" as const,
    sessionId: "ses_original",
    runId: "run_original",
    userItemId: "item_input",
  }));
  const submit = vi.fn<CapturedToolInteraction["submit"]>(async () => ({
    status: "unknown",
    idempotencyKey: "original-input",
    recover,
  }));
  const context: ComponentContext = {
    registerCommand: base.registerCommand,
    subscribeRootRunSettlements: base.subscribeRootRunSettlements,
    notifications: base.notifications,
    component: base.component,
    locale: base.locale,
    signal: base.signal,
    toolResult: base.toolResult,
    get runtime() {
      return base.runtime;
    },
    get telemetry() {
      return base.telemetry;
    },
    saveFile: base.saveFile,
    compose: base.compose,
    track: (work) => work(base.signal),
    interaction: { source: { runId: base.toolResult!.runId, itemId: base.toolResult!.id }, submit },
  };
  testClient.render(<GenUIResult context={context} />);
  const send = screen.getByRole("button", { name: "Send readout" }) as HTMLButtonElement;
  await waitFor(() => expect(send.disabled).toBe(false));
  await act(async () => fireEvent.click(send));
  expect(submit).toHaveBeenCalledOnce();
  expect(submit).toHaveBeenCalledWith({ action: "Review inventory", payloadJSON: '{"search":""}' });
  const recovery = await screen.findByRole("button", { name: "Recover original action" });
  const search = screen.getByRole("searchbox", { name: "Search Inventory" }) as HTMLInputElement;
  fireEvent.change(search, { target: { value: "x".repeat(limits.valueText + 1) } });
  expect(screen.getByText("This field could not be updated.")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Recover original action" })).toBe(recovery);
  expect(screen.getByText(/The original action has not been confirmed/)).toBeTruthy();
  expect(search.value).toBe("");
  expect(send.disabled).toBe(true);
  fireEvent.click(send);
  expect(submit).toHaveBeenCalledOnce();
  fireEvent.change(search, { target: { value: "Memory" } });
  expect(screen.queryByText("This field could not be updated.")).toBeNull();
  expect(send.disabled).toBe(true);
  await act(async () => fireEvent.click(recovery));
  await waitFor(() => expect(recover).toHaveBeenCalledOnce());
  expect(screen.getByText("Action accepted.")).toBeTruthy();
  expect(send.disabled).toBe(false);
  expect(submit).toHaveBeenCalledOnce();
});

it.each([
  { value: 0, max: 4, locale: "en", text: "0%" },
  { value: 3, max: 4, locale: "en", text: "75%" },
  { value: 4, max: 4, locale: "en", text: "100%" },
  { value: 25, locale: "en", text: "25%" },
  { value: 3, max: 4, locale: "de", text: "75\u00a0%" },
])("renders readable progress for $value of $max in $locale", ({ value, max, locale, text }) => {
  show(
    document([
      { id: "coverage", type: "Progress", props: { label: "Review coverage", value, max } },
    ]),
    locale,
  );
  const progress = screen.getByRole("progressbar", { name: "Review coverage" });
  expect(within(progress).getByText("Review coverage")).toBeTruthy();
  expect(within(progress).getByText(text, { normalizer: (value) => value }).hidden).toBe(false);
  expect(progress.getAttribute("aria-valuenow")).toBe(String(value));
  expect(progress.getAttribute("aria-valuemax")).toBe(String(max ?? 100));
  expect(progress.getAttribute("aria-valuetext")).toBe(text);
});

it("updates readable progress through the local control without leaving its editing context", async () => {
  show(
    document(
      [
        {
          id: "scenario",
          type: "Slider",
          props: { field: "covered", label: "Reviewed batches", min: 0, max: 4, step: 1 },
        },
        {
          id: "coverage",
          type: "Progress",
          props: { label: "Review coverage", value: { state: "covered" }, max: 4 },
        },
        { id: "notes", type: "Input", props: { field: "note", label: "Review note" } },
      ],
      [
        { id: "covered", value: 3 },
        { id: "note", value: "Keep the original review." },
      ],
    ),
  );
  await act(async () => {});
  const control = screen.getByRole("slider", { name: "Reviewed batches" });
  const progress = screen.getByRole("progressbar", { name: "Review coverage" });
  act(() => control.focus());
  fireEvent.keyDown(control, { key: "ArrowRight" });
  await waitFor(() => expect(within(progress).getByText("100%")).toBeTruthy());
  expect(progress.getAttribute("aria-valuenow")).toBe("4");
  expect(window.document.activeElement).toBe(control);
  const note = screen.getByRole("textbox", { name: "Review note" });
  act(() => note.focus());
  fireEvent.change(note, { target: { value: "Continue after checking coverage." } });
  expect(window.document.activeElement).toBe(note);
  expect((note as HTMLInputElement).value).toBe("Continue after checking coverage.");
  expect(progress.getAttribute("aria-valuenow")).toBe("4");
});

const report = document([
  { id: "text", type: "Text", props: { text: "Only captured evidence appears in this report." } },
  {
    id: "metric",
    type: "Metric",
    props: { label: "Checks passed", value: "9007199254740993", detail: "Exact recorded count." },
  },
  {
    id: "section",
    type: "Section",
    props: { title: "Follow-up" },
    children: [
      {
        id: "list",
        type: "List",
        props: { items: ["Inspect the admitted package.", "Keep the original evidence."] },
      },
      {
        id: "table",
        type: "Table",
        props: {
          caption: "Feature coverage",
          rowKey: "id",
          columns: [
            { key: "capability", label: "Capability" },
            { key: "status", label: "Status" },
          ],
          rows: [
            { id: "genui", capability: "GenUI", status: "Recorded" },
            { id: "native", capability: "Native use", status: "Pending" },
          ],
        },
      },
    ],
  },
]);

it("renders accepted evidence with precise metric values and accessible section, list and table semantics", async () => {
  show(report);
  const content = screen.getByRole("region", { name: report.title });
  expect(within(content).getByRole("heading", { name: report.title, level: 2 })).toBeTruthy();
  expect(within(content).getByRole("heading", { name: "Follow-up", level: 3 })).toBeTruthy();
  expect(within(content).getByText("Only captured evidence appears in this report.").tagName).toBe(
    "P",
  );
  expect(within(content).getByText("Checks passed").tagName).toBe("DT");
  expect(within(content).getByText("9007199254740993").closest("dd")).toBeTruthy();
  expect(
    within(within(content).getByRole("list"))
      .getAllByRole("listitem")
      .map((item) => item.textContent),
  ).toEqual(["Inspect the admitted package.", "Keep the original evidence."]);
  const table = within(content).getByRole("table", { name: "Feature coverage" });
  expect(
    within(table)
      .getAllByRole("columnheader")
      .map((header) => header.textContent),
  ).toEqual(["Capability", "Status"]);
  expect(
    within(table)
      .getAllByRole("cell")
      .map((cell) => cell.textContent),
  ).toEqual(["GenUI", "Recorded", "Native use", "Pending"]);
  expect(within(content).getByRole("region", { name: "Feature coverage" }).tabIndex).toBe(0);
  await waitFor(() => expect(screen.queryByRole("status")).toBeNull());
});

it("reads only the accepted result and never recovers a document from tool arguments", () => {
  const source = JSON.stringify(report);
  const context = capturedContext(source, "en", {
    args: JSON.stringify({ ...report, title: "Unaccepted request" }),
  });
  testClient.render(<GenUIResult context={context} />);
  expect(screen.getByRole("heading", { name: report.title })).toBeTruthy();
  expect(screen.queryByText("Unaccepted request")).toBeNull();
  expect(Object.isFrozen(context.toolResult)).toBe(true);
  expect(context.toolResult?.result).toBe(source);
});

it("keeps plain display fields literal and confines Markdown to its safe renderer", () => {
  const plain =
    '<script>alert("literal")</script> <img src="https://example.test/pixel" onerror="alert(1)">';
  const action = 'Action("open", "https://example.test") @Set($value, "changed")';
  const { container } = show(
    document([
      { id: "literal", type: "Text", props: { text: plain } },
      {
        id: "action",
        type: "Metric",
        props: { label: "<b>Literal label</b>", value: "0", detail: action },
      },
      {
        id: "markdown",
        type: "Markdown",
        props: {
          text: '**Readable emphasis**\n\n<script>bad()</script>\n\n[Unsafe](javascript:alert(1)) [Safe](https://example.test/review)\n\n```js\nconst exact = "9007199254740993";\n```',
        },
      },
    ]),
  );
  expect(screen.getByText(plain).textContent).toBe(plain);
  expect(screen.getByText(action).textContent).toBe(action);
  expect(screen.getByText("<b>Literal label</b>")).toBeTruthy();
  expect(screen.getByText("Readable emphasis").tagName).toBe("STRONG");
  expect(screen.getByRole("link", { name: "Safe" }).getAttribute("href")).toBe(
    "https://example.test/review",
  );
  expect(screen.queryByRole("link", { name: "Unsafe" })).toBeNull();
  expect(container.querySelector("script, [onclick], [onerror], iframe")).toBeNull();
  expect(container.textContent).toContain('const exact = "9007199254740993";');
});

it("retains a Markdown image failure while an independent local field changes", async () => {
  show(
    document(
      [
        {
          id: "markdown",
          type: "Markdown",
          props: { text: "![Recorded diagram](https://example.test/diagram.png)" },
        },
        { id: "note", type: "Input", props: { field: "note", label: "Review note" } },
      ],
      [{ id: "note", value: "Keep the original note." }],
    ),
  );
  await act(async () => {});
  fireEvent.click(screen.getByRole("button", { name: "Load external content: Recorded diagram" }));
  fireEvent.error(screen.getByRole("img", { name: "Recorded diagram" }));
  const retry = screen.getByRole("button", { name: "Retry" });
  const note = screen.getByRole("textbox", { name: "Review note" });
  act(() => note.focus());
  fireEvent.change(note, { target: { value: "Continue the review." } });
  expect(screen.queryByRole("button", { name: "Retry" })).toBe(retry);
  expect(screen.queryByRole("button", { name: "View image: Recorded diagram" })).toBeNull();
  expect(window.document.activeElement).toBe(note);
});

it("retains Markdown source and table reading positions through an independent edit", async () => {
  show(
    document(
      [
        {
          id: "markdown",
          type: "Markdown",
          props: {
            text: '[Captured guide](https://example.test/guide)\n\n```json\n{"source":"literal  evidence","id":"9007199254740993"}\n```\n\n| Source | Owner |\n| --- | --- |\n| captured | Runtime |',
          },
        },
        { id: "note", type: "Input", props: { field: "note", label: "Review note" } },
      ],
      [{ id: "note", value: "Continue from this record." }],
    ),
  );
  await act(async () => {});
  const source = screen.getByRole("region", { name: "Code" });
  const table = screen.getByRole("region", { name: "Markdown table" });
  const link = screen.getByRole("link", { name: "Captured guide" });
  source.scrollTop = 83;
  source.scrollLeft = 41;
  table.scrollLeft = 57;
  const wrap = source.getAttribute("data-wrap");
  const note = screen.getByRole("textbox", { name: "Review note" });
  act(() => note.focus());
  fireEvent.change(note, { target: { value: "Keep reading after the edit." } });
  expect(screen.getByRole("region", { name: "Code" })).toBe(source);
  expect(screen.getByRole("region", { name: "Markdown table" })).toBe(table);
  expect(screen.getByRole("link", { name: "Captured guide" })).toBe(link);
  expect(source.scrollTop).toBe(83);
  expect(source.scrollLeft).toBe(41);
  expect(table.scrollLeft).toBe(57);
  expect(source.getAttribute("data-wrap")).toBe(wrap);
  expect(window.document.activeElement).toBe(note);
});

it("retains an open Markdown image and its zoom during an independent document update", async () => {
  show(
    document(
      [
        {
          id: "markdown",
          type: "Markdown",
          props: { text: "![Recorded diagram](https://example.test/diagram.png)" },
        },
        { id: "note", type: "Input", props: { field: "note", label: "Review note" } },
      ],
      [{ id: "note", value: "Keep this note." }],
    ),
  );
  await act(async () => {});
  fireEvent.click(screen.getByRole("button", { name: "Load external content: Recorded diagram" }));
  fireEvent.load(screen.getByRole("img", { name: "Recorded diagram" }));
  const trigger = screen.getByRole("button", { name: "View image: Recorded diagram" });
  act(() => trigger.focus());
  fireEvent.click(trigger);
  const preview = await screen.findByRole("dialog", { name: "Recorded diagram" });
  const zoom = within(preview).getByRole("button", { name: "Zoom in image" });
  act(() => zoom.focus());
  fireEvent.click(zoom);
  const viewport = within(preview).getByRole("region", { name: "Recorded diagram" });
  expect(viewport.getAttribute("data-image-zoom")).toBe("125");
  fireEvent.change(screen.getByRole("textbox", { name: "Review note", hidden: true }), {
    target: { value: "The document updated independently." },
  });
  expect(screen.queryByRole("dialog", { name: "Recorded diagram" })).toBe(preview);
  expect(viewport.getAttribute("data-image-zoom")).toBe("125");
  expect(window.document.activeElement).toBe(zoom);
  fireEvent.keyDown(preview, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(window.document.activeElement).toBe(trigger));
  expect(screen.getByRole("textbox", { name: "Review note" })).toHaveProperty(
    "value",
    "The document updated independently.",
  );
});

it("updates Markdown labels without replacing its readers or open image", async () => {
  const catalogs = { en, zh };
  const english = createTranslator(catalogs, "en");
  const chinese = createTranslator(catalogs, "zh");
  const scene = (t: Translate) => (
    <MarkdownView
      props={{
        text: '![Recorded diagram](https://example.test/diagram.png)\n\n```json\n{"source":"captured"}\n```\n\n| Source | Owner |\n| --- | --- |\n| captured | Runtime |',
      }}
      renderNode={() => null}
      t={t}
    />
  );
  const view = testClient.render(scene(english));
  fireEvent.click(screen.getByRole("button", { name: "Load external content: Recorded diagram" }));
  const source = screen.getByRole("region", { name: english("code.label") });
  const table = screen.getByRole("region", { name: english("markdown.table") });
  const trigger = screen.getByRole("button", { name: "View image: Recorded diagram" });
  act(() => trigger.focus());
  fireEvent.click(trigger);
  const preview = await screen.findByRole("dialog", { name: "Recorded diagram" });
  fireEvent.click(within(preview).getByRole("button", { name: english("image.zoomIn") }));
  const viewport = within(preview).getByRole("region", { name: "Recorded diagram" });
  view.rerender(scene(chinese));
  expect(screen.queryByRole("dialog", { name: "Recorded diagram" })).toBe(preview);
  expect(within(preview).getByRole("button", { name: chinese("image.zoomIn") })).toBeTruthy();
  expect(viewport.getAttribute("data-image-zoom")).toBe("125");
  fireEvent.keyDown(preview, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(screen.getByRole("region", { name: chinese("code.label") })).toBe(source);
  expect(screen.getByRole("region", { name: chinese("markdown.table") })).toBe(table);
  expect(screen.getByRole("button", { name: `${chinese("image.view")}: Recorded diagram` })).toBe(
    trigger,
  );
  await waitFor(() => expect(window.document.activeElement).toBe(trigger));
});

it("retires a Markdown image preview when the actual source changes", async () => {
  show(
    document(
      [
        { id: "content", type: "TextArea", props: { field: "content", label: "Markdown source" } },
        { id: "markdown", type: "Markdown", props: { text: { state: "content" } } },
      ],
      [{ id: "content", value: "![Original diagram](https://example.test/original.png)" }],
    ),
  );
  await act(async () => {});
  expect(screen.queryByRole("img")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Load external content: Original diagram" }));
  fireEvent.click(screen.getByRole("button", { name: "View image: Original diagram" }));
  await screen.findByRole("dialog", { name: "Original diagram" });
  fireEvent.change(screen.getByRole("textbox", { name: "Markdown source", hidden: true }), {
    target: { value: "![Next diagram](https://example.test/next.png)" },
  });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(screen.queryByRole("img")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Load external content: Next diagram" }));
  expect(screen.getByRole("img", { name: "Next diagram" }).getAttribute("src")).toBe(
    "https://example.test/next.png",
  );
  fireEvent.change(screen.getByRole("textbox", { name: "Markdown source" }), {
    target: { value: "![Original diagram](https://example.test/original.png)" },
  });
  expect(screen.queryByRole("img")).toBeNull();
  expect(
    screen.getByRole("button", { name: "Load external content: Original diagram" }),
  ).toBeTruthy();
});

it("requires permission for each exact YouTube URL before mounting the external frame", () => {
  const t = createTranslator({ en }, "en");
  const scene = (start: number) => (
    <YouTubeView
      props={{ title: "Recorded walkthrough", videoId: "abcdefghijk", start }}
      renderNode={() => null}
      t={t}
    />
  );
  const { container, rerender } = testClient.render(scene(0));
  expect(container.querySelector("iframe")).toBeNull();
  fireEvent.click(
    screen.getByRole("button", { name: "Load external content: Recorded walkthrough" }),
  );
  expect(container.querySelector("iframe")?.src).toBe(
    "https://www.youtube-nocookie.com/embed/abcdefghijk",
  );
  rerender(scene(15));
  expect(container.querySelector("iframe")).toBeNull();
  rerender(scene(0));
  expect(container.querySelector("iframe")).toBeNull();
});

it.each([
  ["en", "No rows to display."],
  ["zh", "没有可显示的数据行。"],
  ["zh-TW", "沒有可顯示的資料列。"],
  ["ja", "表示する行がありません。"],
  ["ko", "표시할 행이 없습니다."],
  ["es", "No hay filas para mostrar."],
  ["fr", "Aucune ligne à afficher."],
  ["de", "Keine Zeilen zum Anzeigen."],
])("presents empty data in %s without inventing records", (locale, message) => {
  show(
    document([
      {
        id: "table",
        type: "Table",
        props: {
          caption: "Inventory",
          rowKey: "id",
          columns: [
            { key: "item", label: "Item" },
            { key: "quantity", label: "Quantity" },
          ],
          rows: [],
        },
      },
    ]),
    locale,
  );
  const table = screen.getByRole("table", { name: "Inventory" });
  const cell = within(table).getByRole("cell");
  expect(cell.textContent).toBe(message);
  expect(cell.getAttribute("colspan")).toBe("2");
});

it("uses the same Store for number lexemes, pure expressions, form validation, submit and reset", async () => {
  const { context } = show(GENUI_REPORT);
  const input = screen.getByRole("textbox", { name: "Delivery weeks" });
  await waitFor(() => expect((input as HTMLInputElement).value).toBe("4"));
  const value = () => screen.getByText("Projected checks").nextElementSibling?.textContent;
  expect(value()).toContain("48");
  fireEvent.change(input, { target: { value: "-" } });
  expect((input as HTMLInputElement).value).toBe("-");
  expect(screen.getByText("Enter a valid number.")).toBeTruthy();
  expect(value()).toContain("0");
  await act(async () => {
    fireEvent.submit(screen.getByRole("form", { name: "Delivery scenario" }));
  });
  expect(screen.getByText("No scenario has been applied.")).toBeTruthy();
  fireEvent.change(input, { target: { value: "6" } });
  fireEvent.change(screen.getByRole("textbox", { name: "Delivery team" }), {
    target: { value: "Review team" },
  });
  expect(value()).toContain("72");
  await act(async () => {
    fireEvent.submit(screen.getByRole("form", { name: "Delivery scenario" }));
  });
  await screen.findByText("Applied scenario for Review team: 6 delivery weeks.");
  fireEvent.click(screen.getByRole("button", { name: "Reset scenario" }));
  expect((input as HTMLInputElement).value).toBe("4");
  expect((screen.getByRole("textbox", { name: "Delivery team" }) as HTMLInputElement).value).toBe(
    "Runtime team",
  );
  expect(value()).toContain("48");
  expect(context.toolResult?.result).toBe(JSON.stringify(GENUI_REPORT));
});

it("composes controls, conditional views and repeated nodes from their declared local fields", async () => {
  show(GENUI_REPORT);
  await waitFor(() =>
    expect(
      (screen.getByRole("textbox", { name: "Delivery weeks" }) as HTMLInputElement).value,
    ).toBe("4"),
  );
  fireEvent.click(screen.getByRole("button", { name: "Advanced scenario controls" }));
  fireEvent.click(screen.getByLabelText("Computer use", { selector: "input" }));
  fireEvent.click(screen.getByLabelText("Ready for a review", { selector: "input" }));
  expect(
    screen.getByRole("checkbox", { name: "Ready for a review" }).getAttribute("aria-checked"),
  ).toBe("true");
  const radio = screen.getByRole("radio", { name: "Internal" });
  fireEvent.keyDown(radio, { key: "ArrowRight" });
  expect(screen.getByRole("radio", { name: "Public" }).getAttribute("aria-checked")).toBe("true");
  fireEvent.change(screen.getByLabelText("Review date"), { target: { value: "2026-11-15" } });
  expect((screen.getByLabelText("Review date") as HTMLInputElement).value).toBe("2026-11-15");
  const toggle = screen.getByRole("switch", { name: "Show the local projection" });
  fireEvent.click(toggle);
  expect(
    screen.queryByText(
      "Scenario changes are local to this view. The captured review remains available in the inspector.",
    ),
  ).toBeNull();
  fireEvent.click(screen.getByRole("tab", { name: "Explore displays" }));
  const panel = screen.getByRole("tabpanel", { name: "Explore displays" });
  expect(within(panel).getByText("genui", { exact: true })).toBeTruthy();
  expect(within(panel).getByText("computer", { exact: true })).toBeTruthy();
  expect(within(panel).queryByText("code", { exact: true })).toBeNull();
  expect(within(panel).getByRole("img", { name: "Delivery progress illustration" })).toBeTruthy();
  expect(panel.querySelector("math")).toBeTruthy();
  fireEvent.click(within(panel).getByRole("button", { name: "Show answer" }));
  expect(within(panel).getByText("Runtime owns execution and recovery.")).toBeTruthy();
  fireEvent.click(within(panel).getByRole("button", { name: "Review hint" }));
  expect(screen.getByRole("dialog", { name: "Review hint" })).toBeTruthy();
  fireEvent.keyDown(screen.getByRole("dialog", { name: "Review hint" }), { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog", { name: "Review hint" })).toBeNull());
});

function popoverGeometry() {
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (
    this: HTMLElement,
  ) {
    if (this.closest("[hidden]")) return new DOMRect();
    return this === window.document.documentElement || this === window.document.body
      ? new DOMRect(0, 0, 1024, 768)
      : new DOMRect(20, 20, 160, 32);
  });
  for (const [property, field, viewport] of [
    ["clientWidth", 160, 1024],
    ["clientHeight", 32, 768],
  ] as const) {
    vi.spyOn(HTMLElement.prototype, property, "get").mockImplementation(function (
      this: HTMLElement,
    ) {
      return this === window.document.documentElement || this === window.document.body
        ? viewport
        : field;
    });
  }
}

async function popoverScene() {
  popoverGeometry();
  const context = capturedContext(
    JSON.stringify(
      document(
        [
          {
            id: "review",
            type: "Popover",
            props: { field: "open", label: "Review hint" },
            children: [
              { id: "note", type: "Input", props: { field: "note", label: "Review note" } },
              {
                id: "mode",
                type: "Select",
                props: {
                  field: "mode",
                  label: "Review mode",
                  options: [
                    { value: "balanced", label: "Balanced" },
                    { value: "focused", label: "Focused" },
                  ],
                },
              },
              {
                id: "guidance",
                type: "Popover",
                props: { field: "nested", label: "More guidance" },
                children: [
                  { id: "detail", type: "Input", props: { field: "detail", label: "Second note" } },
                  {
                    id: "finish",
                    type: "Button",
                    props: { label: "Back to report", action: "close" },
                  },
                ],
              },
            ],
          },
        ],
        [
          { id: "open", value: false },
          { id: "nested", value: false },
          { id: "note", value: "Keep this review note." },
          { id: "detail", value: "Keep the nested note." },
          { id: "mode", value: "balanced" },
        ],
        [{ id: "close", steps: [{ type: "set", state: "open", value: { literal: false } }] }],
      ),
    ),
  );
  const frame = (visible: boolean) => (
    <>
      <input aria-label="Later draft" defaultValue="Keep authoring" />
      <div hidden={!visible}>
        <ui.ContentVisibility visible={visible}>
          <GenUIResult context={context} />
        </ui.ContentVisibility>
      </div>
    </>
  );
  const view = testClient.render(frame(true));
  await act(async () => {});
  return { setVisible: (visible: boolean) => view.rerender(frame(visible)) };
}

it("closes only the innermost generated popover with Escape and retains its local draft", async () => {
  await popoverScene();
  fireEvent.click(screen.getByRole("button", { name: "Review hint" }));
  fireEvent.click(screen.getByRole("button", { name: "More guidance" }));
  const detail = screen.getByRole("textbox", { name: "Second note" });
  fireEvent.change(detail, { target: { value: "Edited nested note" } });
  act(() => detail.focus());

  fireEvent.keyDown(detail, { key: "Escape" });

  await waitFor(() => expect(screen.queryByRole("dialog", { name: "More guidance" })).toBeNull());
  expect(screen.getByRole("dialog", { name: "Review hint" })).toBeTruthy();
  await waitFor(() =>
    expect(window.document.activeElement).toBe(
      screen.getByRole("button", { name: "More guidance" }),
    ),
  );
  fireEvent.click(screen.getByRole("button", { name: "More guidance" }));
  expect((screen.getByRole("textbox", { name: "Second note" }) as HTMLInputElement).value).toBe(
    "Edited nested note",
  );
});

it("keeps a generated popover open while choosing from its portaled Select", async () => {
  await popoverScene();
  fireEvent.click(screen.getByRole("button", { name: "Review hint" }));
  const mode = screen.getByRole("combobox", { name: "Review mode" });
  act(() => mode.focus());
  fireEvent.click(mode);
  const list = await screen.findByRole("listbox");
  const option = within(list).getByRole("option", { name: "Focused" });

  fireEvent.pointerDown(option, { pointerType: "mouse" });
  fireEvent.click(option);

  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  expect(screen.getByRole("dialog", { name: "Review hint" })).toBeTruthy();
  expect(mode.textContent).toContain("Focused");
  await waitFor(() => expect(window.document.activeElement).toBe(mode));
});

it("retires nested generated details when a declared action closes their parent", async () => {
  await popoverScene();
  fireEvent.click(screen.getByRole("button", { name: "Review hint" }));
  fireEvent.click(screen.getByRole("button", { name: "More guidance" }));
  fireEvent.change(screen.getByRole("textbox", { name: "Second note" }), {
    target: { value: "Edited nested note" },
  });

  fireEvent.click(screen.getByRole("button", { name: "Back to report" }));

  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  fireEvent.click(screen.getByRole("button", { name: "Review hint" }));
  expect(screen.queryByRole("dialog", { name: "More guidance" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "More guidance" }));
  expect((screen.getByRole("textbox", { name: "Second note" }) as HTMLInputElement).value).toBe(
    "Edited nested note",
  );
});

it("keeps generated sandbox content out of a closed popover's lifetime", async () => {
  show(
    document(
      [
        {
          id: "preview",
          type: "Popover",
          props: { field: "open", label: "Local preview" },
          children: [
            {
              id: "demo",
              type: "HtmlView",
              props: { title: "Local demonstration", html: "<p>Captured demonstration</p>" },
            },
          ],
        },
      ],
      [{ id: "open", value: false }],
    ),
  );
  await act(async () => {});
  expect(screen.queryByTitle("Local demonstration")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Local preview" }));
  const frame = screen.getByTitle("Local demonstration");
  expect(frame.getAttribute("sandbox")).toBe("allow-scripts");

  fireEvent.keyDown(screen.getByRole("dialog", { name: "Local preview" }), { key: "Escape" });

  await waitFor(() => expect(frame.isConnected).toBe(false));
});

it("retires a hidden generated popover without reopening it over a later draft", async () => {
  const scene = await popoverScene();
  fireEvent.click(screen.getByRole("button", { name: "Review hint" }));
  fireEvent.change(screen.getByRole("textbox", { name: "Review note" }), {
    target: { value: "Edited review note" },
  });
  const draft = screen.getByRole("textbox", { name: "Later draft" });

  scene.setVisible(false);
  act(() => draft.focus());
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  scene.setVisible(true);

  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(window.document.activeElement).toBe(draft);
  expect((draft as HTMLInputElement).value).toBe("Keep authoring");
  fireEvent.click(screen.getByRole("button", { name: "Review hint" }));
  expect((screen.getByRole("textbox", { name: "Review note" }) as HTMLInputElement).value).toBe(
    "Edited review note",
  );
});

it("dismisses a generated popover on an outside pointer without retargeting later authoring", async () => {
  await popoverScene();
  fireEvent.click(screen.getByRole("button", { name: "Review hint" }));
  fireEvent.change(screen.getByRole("textbox", { name: "Review note" }), {
    target: { value: "Edited review note" },
  });
  const draft = screen.getByRole("textbox", { name: "Later draft" });
  fireEvent.pointerDown(draft, { pointerType: "mouse" });
  fireEvent.mouseDown(draft);
  act(() => draft.focus());
  fireEvent.pointerUp(draft, { pointerType: "mouse" });
  fireEvent.mouseUp(draft);
  fireEvent.click(draft);
  fireEvent.change(draft, { target: { value: "Continue authoring" } });

  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(window.document.activeElement).toBe(draft);
  expect((draft as HTMLInputElement).value).toBe("Continue authoring");
  fireEvent.click(screen.getByRole("button", { name: "Review hint" }));
  expect((screen.getByRole("textbox", { name: "Review note" }) as HTMLInputElement).value).toBe(
    "Edited review note",
  );
});

const popoverTable = document(
  [
    {
      id: "details",
      type: "Popover",
      props: { field: "open", label: "Device details" },
      children: [
        {
          id: "devices",
          type: "Table",
          props: {
            caption: "Devices",
            rowKey: "id",
            columns: [{ key: "name", label: "Name" }],
            rows: [
              { id: "router", name: "Router" },
              { id: "relay", name: "Relay" },
              { id: "switch", name: "Switch" },
            ],
            searchState: "search",
            pageState: "page",
            selectState: "selected",
            pageSize: 1,
          },
        },
      ],
    },
  ],
  [
    { id: "open", value: false },
    { id: "search", value: "" },
    { id: "page", value: 0 },
    { id: "selected", value: [] },
  ],
);

async function openPopoverTable() {
  popoverGeometry();
  show(popoverTable);
  await act(async () => {});
  fireEvent.click(screen.getByRole("button", { name: "Device details" }));
  const dialog = screen.getByRole("dialog", { name: "Device details" });
  return {
    dialog,
    search: within(dialog).getByRole("searchbox", { name: "Search Devices" }) as HTMLInputElement,
    page: () =>
      within(within(dialog).getByRole("group", { name: "Devices pages" })).getByRole("status")
        .textContent,
  };
}

function toggleTableRow(table: HTMLElement, name: string) {
  const checkbox = within(table).getByRole("checkbox", { name });
  // happy-dom activates the label before React can cancel the mark's click.
  // Use its native activation once; Wails acceptance also covers the visible mark.
  fireEvent.click(checkbox.closest("label")!);
}

it("clears a popover table query with Escape before the popover dismisses", async () => {
  const { search, page } = await openPopoverTable();
  fireEvent.change(search, { target: { value: "no such device" } });
  act(() => search.focus());

  fireEvent.keyDown(search, { key: "Escape" });

  expect(screen.getByRole("dialog", { name: "Device details" })).toBeTruthy();
  expect(search.value).toBe("");
  expect(window.document.activeElement).toBe(search);
  expect(page()).toBe("1 of 3");

  fireEvent.keyDown(search, { key: "Escape" });

  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() =>
    expect(window.document.activeElement).toBe(
      screen.getByRole("button", { name: "Device details" }),
    ),
  );
  fireEvent.click(screen.getByRole("button", { name: "Device details" }));
  expect(
    (screen.getByRole("searchbox", { name: "Search Devices" }) as HTMLInputElement).value,
  ).toBe("");
});

it("keeps popover table page bounds focusable and inert inside the popover", async () => {
  const { dialog, search, page } = await openPopoverTable();
  const table = within(dialog).getByRole("table", { name: "Devices" });
  const pagination = within(dialog).getByRole("group", { name: "Devices pages" });
  const previous = within(pagination).getByRole("button", { name: "Previous" });
  const next = within(pagination).getByRole("button", { name: "Next" });
  const atBound = (bound: HTMLElement, expected: string, row: string) => {
    act(() => search.focus());
    act(() => bound.focus());
    expect(window.document.activeElement).toBe(bound);
    expect(bound.getAttribute("aria-disabled")).toBe("true");
    fireEvent.click(bound);
    fireEvent.keyDown(bound, { key: "Enter" });
    fireEvent.keyDown(bound, { key: " " });
    fireEvent.keyUp(bound, { key: " " });
    expect(page()).toBe(expected);
    expect(within(table).getByRole("cell", { name: row })).toBeTruthy();
    expect(window.document.activeElement).toBe(bound);
  };

  fireEvent.click(next);
  fireEvent.click(next);
  toggleTableRow(table, "Select switch");
  atBound(next, "3 of 3", "Switch");
  expect(
    within(table).getByRole("checkbox", { name: "Select switch" }).getAttribute("aria-checked"),
  ).toBe("true");

  fireEvent.click(previous);
  fireEvent.click(previous);
  atBound(previous, "1 of 3", "Router");

  fireEvent.keyDown(previous, { key: "Escape" });

  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() =>
    expect(window.document.activeElement).toBe(
      screen.getByRole("button", { name: "Device details" }),
    ),
  );
});

it("clears a popover table query by pointer in its document reading grammar", async () => {
  const original = JSON.stringify(popoverTable);
  const { dialog, search, page } = await openPopoverTable();
  const table = within(dialog).getByRole("table", { name: "Devices" });
  expect(table.closest('[data-slot="document-body"]')).not.toBeNull();
  fireEvent.change(search, { target: { value: "R" } });
  const pagination = within(dialog).getByRole("group", { name: "Devices pages" });
  fireEvent.click(within(pagination).getByRole("button", { name: "Next" }));
  expect(page()).toBe("2 of 2");
  toggleTableRow(table, "Select relay");

  fireEvent.click(within(dialog).getByRole("button", { name: "Clear search" }));

  expect(screen.getByRole("dialog", { name: "Device details" })).toBe(dialog);
  expect(search.value).toBe("");
  expect(window.document.activeElement).toBe(search);
  expect(page()).toBe("1 of 3");
  expect(within(table).getByRole("cell", { name: "Router" })).toBeTruthy();
  fireEvent.click(within(pagination).getByRole("button", { name: "Next" }));
  expect(
    within(table).getByRole("checkbox", { name: "Select relay" }).getAttribute("aria-checked"),
  ).toBe("true");
  expect(JSON.stringify(popoverTable)).toBe(original);
});

it("names table selection by stable row identifier without reading it as table text", async () => {
  const alpha = "evidence-runtime-receipt-7f3c9a1e5b2d4c6f8a0b1c2d3e4f5a6b";
  const beta = "evidence-runtime-receipt-1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d";
  const ledger = document(
    [
      {
        id: "details",
        type: "Popover",
        props: { field: "open", label: "Ledger details" },
        children: [
          {
            id: "ledger",
            type: "Table",
            props: {
              caption: "Ledger",
              rowKey: "id",
              columns: [{ key: "name", label: "Evidence" }],
              rows: [
                { id: beta, name: "Beta" },
                { id: alpha, name: "Alpha" },
              ],
              searchState: "search",
              sortState: "sort",
              pageState: "page",
              selectState: "selected",
              pageSize: 1,
            },
          },
        ],
      },
    ],
    [
      { id: "open", value: false },
      { id: "search", value: "" },
      { id: "sort", value: "" },
      { id: "page", value: 0 },
      { id: "selected", value: [] },
    ],
  );
  const original = JSON.stringify(ledger);
  popoverGeometry();
  show(ledger);
  await act(async () => {});
  const open = () => {
    fireEvent.click(screen.getByRole("button", { name: "Ledger details" }));
    const dialog = screen.getByRole("dialog", { name: "Ledger details" });
    return { dialog, table: within(dialog).getByRole("table", { name: "Ledger" }) };
  };
  const selection = (table: HTMLElement, row: string) =>
    within(table).getByRole("checkbox", { name: `Select ${row}` });

  let { dialog, table } = open();
  fireEvent.click(within(table).getByRole("button", { name: "Sort by Evidence" }));
  expect(within(table).getByRole("cell", { name: "Alpha" })).toBeTruthy();
  toggleTableRow(table, `Select ${alpha}`);
  expect(selection(table, alpha).getAttribute("aria-checked")).toBe("true");
  expect(selection(table, alpha).closest("tr")?.getAttribute("aria-selected")).toBe("true");
  expect(selection(table, alpha).closest("td")?.textContent).toBe("");
  expect(table.textContent).not.toContain(alpha);

  const search = within(dialog).getByRole("searchbox", { name: "Search Ledger" });
  fireEvent.change(search, { target: { value: "Beta" } });
  expect(selection(table, beta).getAttribute("aria-checked")).toBe("false");
  fireEvent.change(search, { target: { value: "" } });
  expect(selection(table, alpha).getAttribute("aria-checked")).toBe("true");
  const next = within(dialog).getByRole("button", { name: "Next" });
  fireEvent.click(next);
  expect(within(table).getByRole("cell", { name: "Beta" })).toBeTruthy();
  expect(table.textContent).not.toContain(beta);

  act(() => next.focus());
  fireEvent.keyDown(next, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  ({ dialog, table } = open());
  fireEvent.click(within(dialog).getByRole("button", { name: "Previous" }));
  expect(selection(table, alpha).getAttribute("aria-checked")).toBe("true");
  expect(table.textContent).not.toContain(alpha);
  expect(JSON.stringify(ledger)).toBe(original);
});

it("filters, sorts, pages and selects rows without changing the captured data", async () => {
  const original = JSON.stringify(GENUI_REPORT);
  show(GENUI_REPORT);
  const table = screen.getByRole("table", { name: "Plugin delivery responsibilities" });
  await waitFor(() => expect(within(table).getAllByRole("row")).toHaveLength(3));
  const pagination = screen.getByRole("group", { name: "Plugin delivery responsibilities pages" });
  fireEvent.click(within(pagination).getByRole("button", { name: "Next" }));
  expect(within(table).getByRole("cell", { name: "Browser automation" })).toBeTruthy();
  toggleTableRow(table, "Select browser");
  expect(screen.getByText("Selected evidence rows").nextElementSibling?.textContent).toBe("1");
  const search = screen.getByRole("searchbox", { name: "Search Plugin delivery responsibilities" });
  fireEvent.change(search, { target: { value: "Exact identifiers" } });
  expect(within(table).getAllByRole("row")).toHaveLength(2);
  expect(within(table).getAllByRole("cell", { name: "9007199254740993" })).toHaveLength(2);
  fireEvent.change(search, { target: { value: "" } });
  fireEvent.click(within(table).getByRole("button", { name: "Sort by Capability" }));
  expect(within(table).getAllByRole("row")[1]?.textContent).toContain("Browser automation");
  expect(
    within(table)
      .getByRole("columnheader", { name: /Capability/ })
      .getAttribute("aria-sort"),
  ).toBe("ascending");
  expect(
    within(table).getByRole("checkbox", { name: "Select browser" }).getAttribute("aria-checked"),
  ).toBe("true");
  fireEvent.change(search, { target: { value: "no such evidence" } });
  expect(within(table).getByRole("cell").textContent).toBe("No rows to display.");
  expect(JSON.stringify(GENUI_REPORT)).toBe(original);
});

it.each([false, true])(
  "names the next flashcard action without an inverted toggle state from %s",
  async (flipped) => {
    show(
      document(
        [
          {
            id: "card",
            type: "Flashcard",
            props: {
              field: "flipped",
              front: "Who owns execution receipts?",
              back: "Runtime owns execution and recovery.",
            },
          },
        ],
        [{ id: "flipped", value: flipped }],
      ),
    );
    await act(async () => {});
    const card = screen.getByRole("region", { name: "Flashcard" });
    const action = within(card).getByRole("button", {
      name: flipped ? "Show question" : "Show answer",
    });
    expect(action.getAttribute("aria-pressed")).toBeNull();
    act(() => action.focus());
    fireEvent.click(action);
    expect(
      within(card).getByRole("button", { name: flipped ? "Show answer" : "Show question" }),
    ).toBe(action);
    expect(action.getAttribute("aria-pressed")).toBeNull();
    expect(
      within(card).getByText(
        flipped ? "Who owns execution receipts?" : "Runtime owns execution and recovery.",
      ),
    ).toBeTruthy();
    expect(window.document.activeElement).toBe(action);
  },
);

it("owns timer values in the same Store and releases its clock on unmount", async () => {
  const value = document(
    [
      {
        id: "clock",
        type: "Timer",
        props: { field: "seconds", runningState: "running", label: "Review clock" },
      },
    ],
    [
      { id: "seconds", value: 3 },
      { id: "running", value: false },
    ],
  );
  const rendered = show(value);
  await waitFor(() => expect(screen.getByRole("timer").textContent).toBe("0:03"));
  vi.useFakeTimers();
  fireEvent.click(screen.getByRole("button", { name: "Start" }));
  await act(async () => {
    vi.advanceTimersByTime(1250);
  });
  expect(screen.getByRole("timer").textContent).toBe("0:02");
  fireEvent.click(screen.getByRole("button", { name: "Pause" }));
  await act(async () => {
    vi.advanceTimersByTime(1000);
  });
  expect(screen.getByRole("timer").textContent).toBe("0:02");
  fireEvent.click(screen.getByRole("button", { name: "Reset" }));
  expect(screen.getByRole("timer").textContent).toBe("0:03");
  fireEvent.click(screen.getByRole("button", { name: "Start" }));
  rendered.unmount();
  expect(vi.getTimerCount()).toBe(0);
});

function tabReadingDocument() {
  return document(
    [
      { id: "note", type: "Input", props: { field: "note", label: "Review note" } },
      {
        id: "views",
        type: "Tabs",
        props: {
          field: "panel",
          items: [
            { value: "evidence", label: "Evidence" },
            { value: "source", label: "Captured source" },
            { value: "options", label: "Review options" },
          ],
        },
        children: [
          { id: "evidence", type: "Text", props: { text: "Read the accepted result." } },
          { id: "source", type: "Code", props: { code: "literal  captured source" } },
          {
            id: "options",
            type: "RadioGroup",
            props: {
              field: "audience",
              label: "Review audience",
              options: [
                { value: "internal", label: "Internal review" },
                { value: "public", label: "Public review" },
              ],
            },
          },
        ],
      },
    ],
    [
      { id: "note", value: "Keep this note." },
      { id: "panel", value: "evidence" },
      { id: "audience", value: "internal" },
    ],
  );
}

it.each(["ArrowUp", "ArrowDown"])(
  "leaves %s available for reading while a horizontal report tab is focused",
  async (key) => {
    show(tabReadingDocument());
    await act(async () => {});
    const source = screen.getByRole("tab", { name: "Captured source" });
    fireEvent.click(source);
    act(() => source.focus());
    expect(fireEvent.keyDown(source, { key, cancelable: true })).toBe(true);
    expect(source.getAttribute("aria-selected")).toBe("true");
    expect(screen.getByRole("tabpanel", { name: "Captured source" })).toBeTruthy();
    expect(window.document.activeElement).toBe(source);
  },
);

it("keeps horizontal report tab selection, wrapping and local drafts together", async () => {
  show(tabReadingDocument());
  await act(async () => {});
  const note = screen.getByRole("textbox", { name: "Review note" });
  fireEvent.change(note, { target: { value: "Continue after reading." } });
  const evidence = screen.getByRole("tab", { name: "Evidence" });
  const source = screen.getByRole("tab", { name: "Captured source" });
  const options = screen.getByRole("tab", { name: "Review options" });
  act(() => evidence.focus());
  for (const [key, target] of [
    ["ArrowRight", source],
    ["End", options],
    ["ArrowRight", evidence],
    ["ArrowLeft", options],
    ["Home", evidence],
  ] as const) {
    expect(fireEvent.keyDown(window.document.activeElement!, { key, cancelable: true })).toBe(
      false,
    );
    expect(window.document.activeElement).toBe(target);
    expect(target.getAttribute("aria-selected")).toBe("true");
    expect(screen.getAllByRole("tab").filter((tab) => tab.tabIndex === 0)).toEqual([target]);
  }
  expect(note).toHaveProperty("value", "Continue after reading.");
  expect(screen.getByRole("tabpanel", { name: "Evidence" })).toBeTruthy();
});

it("keeps vertical arrows selecting a report radio choice", async () => {
  show(tabReadingDocument());
  await act(async () => {});
  fireEvent.click(screen.getByRole("tab", { name: "Review options" }));
  const internal = screen.getByRole("radio", { name: "Internal review" });
  const external = screen.getByRole("radio", { name: "Public review" });
  act(() => internal.focus());
  expect(fireEvent.keyDown(internal, { key: "ArrowDown", cancelable: true })).toBe(false);
  expect(window.document.activeElement).toBe(external);
  expect(external.getAttribute("aria-checked")).toBe("true");
  expect(fireEvent.keyDown(external, { key: "ArrowUp", cancelable: true })).toBe(false);
  expect(window.document.activeElement).toBe(internal);
  expect(internal.getAttribute("aria-checked")).toBe("true");
});

it("renders controlled accordion and carousel panels with keyboard selection", async () => {
  show(
    document(
      [
        {
          id: "accordion",
          type: "Accordion",
          props: {
            field: "open",
            items: [
              { value: "first", label: "First detail" },
              { value: "second", label: "Second detail" },
            ],
          },
          children: [
            { id: "one", type: "Text", props: { text: "First content" } },
            { id: "two", type: "Text", props: { text: "Second content" } },
          ],
        },
        {
          id: "carousel",
          type: "Carousel",
          props: { field: "slide" },
          children: [
            { id: "slideOne", type: "Text", props: { text: "Slide one" } },
            { id: "slideTwo", type: "Text", props: { text: "Slide two" } },
          ],
        },
      ],
      [
        { id: "open", value: [] },
        { id: "slide", value: 0 },
      ],
    ),
  );
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "First detail" }).getAttribute("aria-expanded")).toBe(
      "false",
    ),
  );
  fireEvent.click(screen.getByRole("button", { name: "Second detail" }));
  expect(screen.getByRole("button", { name: "Second detail" }).getAttribute("aria-expanded")).toBe(
    "true",
  );
  fireEvent.click(screen.getByRole("button", { name: "Next" }));
  expect(screen.getByText("Slide one").closest("[hidden]")).toBeTruthy();
  expect(screen.getByText("Slide two").closest("[hidden]")).toBeNull();
  expect(screen.getByRole("button", { name: "Next" }).getAttribute("aria-disabled")).toBe("true");
});

it("keeps carousel page bounds focusable and inert inside a popover", async () => {
  popoverGeometry();
  const value = document(
    [
      {
        id: "slides",
        type: "Popover",
        props: { field: "open", label: "Review slides" },
        children: [
          {
            id: "carousel",
            type: "Carousel",
            props: { field: "slide" },
            children: [
              { id: "noteSlide", type: "Input", props: { field: "note", label: "Slide note" } },
              { id: "middleSlide", type: "Text", props: { text: "Slide two" } },
              { id: "lastSlide", type: "Text", props: { text: "Slide three" } },
            ],
          },
        ],
      },
    ],
    [
      { id: "open", value: false },
      { id: "slide", value: 0 },
      { id: "note", value: "Captured note" },
    ],
  );
  const original = JSON.stringify(value);
  show(value);
  await act(async () => {});
  fireEvent.click(screen.getByRole("button", { name: "Review slides" }));
  const dialog = screen.getByRole("dialog", { name: "Review slides" });
  const note = within(dialog).getByRole("textbox", { name: "Slide note" }) as HTMLInputElement;
  fireEvent.change(note, { target: { value: "Edited slide note" } });
  const previous = within(dialog).getByRole("button", { name: "Previous" });
  const next = within(dialog).getByRole("button", { name: "Next" });
  const page = () => within(dialog).getByRole("status").textContent;
  const atBound = (bound: HTMLElement, expected: string, visible: HTMLElement) => {
    act(() => bound.blur());
    act(() => bound.focus());
    expect(window.document.activeElement).toBe(bound);
    expect(bound.getAttribute("aria-disabled")).toBe("true");
    fireEvent.click(bound);
    fireEvent.keyDown(bound, { key: "Enter" });
    fireEvent.keyDown(bound, { key: " " });
    fireEvent.keyUp(bound, { key: " " });
    expect(page()).toBe(expected);
    expect(visible.closest("[hidden]")).toBeNull();
    expect(window.document.activeElement).toBe(bound);
  };

  atBound(previous, "1 of 3", note);
  fireEvent.click(next);
  fireEvent.click(next);
  atBound(next, "3 of 3", within(dialog).getByText("Slide three"));
  expect(note.closest("[hidden]")).toBeTruthy();
  expect(note.value).toBe("Edited slide note");

  fireEvent.click(previous);
  fireEvent.click(previous);
  atBound(previous, "1 of 3", note);
  expect(note.value).toBe("Edited slide note");

  fireEvent.keyDown(previous, { key: "Escape" });

  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() =>
    expect(window.document.activeElement).toBe(
      screen.getByRole("button", { name: "Review slides" }),
    ),
  );
  expect(JSON.stringify(value)).toBe(original);
});

it("shows map tile failures without hiding location data and retries only when requested", async () => {
  const value = document(
    [
      {
        id: "map",
        type: "Map",
        props: {
          caption: "Review locations",
          selectedState: "place",
          markers: [
            {
              id: "london",
              label: "London",
              latitude: 51.5074,
              longitude: -0.1278,
              description: "Review meeting",
            },
            { id: "paris", label: "Paris", latitude: 48.8566, longitude: 2.3522 },
          ],
          route: [
            { latitude: 51.5074, longitude: -0.1278 },
            { latitude: 48.8566, longitude: 2.3522 },
          ],
        },
      },
    ],
    [{ id: "place", value: "" }],
  );
  const { container } = show(value);
  expect(container.querySelector('img[src*="tile.openstreetmap.org"]')).toBeNull();
  expect(screen.getByRole("table", { name: "Locations in Review locations" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Load external content: Review locations" }));
  const image = await waitFor(() => {
    const image = container.querySelector<HTMLImageElement>('img[src*="tile.openstreetmap.org"]');
    expect(image).toBeTruthy();
    return image!;
  });
  fireEvent.error(image);
  expect(screen.getByRole("alert").textContent).toContain("Some map tiles could not be loaded.");
  expect(screen.getByRole("table", { name: "Locations in Review locations" })).toBeTruthy();
  expect(screen.getByRole("link", { name: "OpenStreetMap" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Retry" }));
  expect(image.isConnected).toBe(false);
  expect(screen.queryByRole("alert")).toBeNull();
  const places = screen.getByRole("table", { name: "Locations in Review locations" });
  fireEvent.click(within(places).getByRole("button", { name: "Paris" }));
  expect(
    within(places)
      .getByRole("button", { name: "Paris" })
      .closest("tr")
      ?.getAttribute("aria-selected"),
  ).toBe("true");
  expect(screen.getByRole("button", { name: "Zoom in" })).toBeTruthy();
});

it("keeps map zoom bounds focusable and offers marker controls only inside the viewport", async () => {
  const value = document(
    [
      {
        id: "map",
        type: "Map",
        props: {
          caption: "Review locations",
          selectedState: "place",
          markers: [
            { id: "london", label: "London", latitude: 51.5074, longitude: -0.1278 },
            { id: "paris", label: "Paris", latitude: 48.8566, longitude: 2.3522 },
          ],
        },
      },
    ],
    [{ id: "place", value: "" }],
  );
  const original = JSON.stringify(value);
  show(value);
  await act(async () => {});
  fireEvent.click(screen.getByRole("button", { name: "Load external content: Review locations" }));
  const places = screen.getByRole("table", { name: "Locations in Review locations" });
  const marker = (name: string, pressed: boolean) =>
    screen.queryByRole("button", { name, pressed });
  const row = (name: string) =>
    within(places).getByRole("button", { name }).closest("tr")?.getAttribute("aria-selected");
  const zoomIn = screen.getByRole("button", { name: "Zoom in" });
  const zoomOut = screen.getByRole("button", { name: "Zoom out" });
  const toBound = (bound: HTMLElement) => {
    for (let step = 0; step < 20 && bound.getAttribute("aria-disabled") !== "true"; step++) {
      fireEvent.click(bound);
    }
    act(() => bound.blur());
    act(() => bound.focus());
    expect(window.document.activeElement).toBe(bound);
    expect(bound.getAttribute("aria-disabled")).toBe("true");
    fireEvent.click(bound);
    fireEvent.keyDown(bound, { key: "Enter" });
    fireEvent.keyDown(bound, { key: " " });
    fireEvent.keyUp(bound, { key: " " });
    for (const key of ["Tab", "ArrowDown", "Escape"]) {
      expect(fireEvent.keyDown(bound, { key, cancelable: true })).toBe(true);
    }
    expect(bound.getAttribute("aria-disabled")).toBe("true");
    expect(window.document.activeElement).toBe(bound);
  };

  fireEvent.click(marker("London", false)!);
  expect(marker("London", true)).toBeTruthy();
  expect(marker("Paris", false)).toBeTruthy();

  toBound(zoomIn);
  expect(zoomOut.getAttribute("aria-disabled")).not.toBe("true");
  for (const name of ["London", "Paris"]) {
    expect(marker(name, true)).toBeNull();
    expect(marker(name, false)).toBeNull();
  }
  expect(row("London")).toBe("true");
  fireEvent.click(within(places).getByRole("button", { name: "Paris" }));
  expect(row("Paris")).toBe("true");
  fireEvent.click(within(places).getByRole("button", { name: "London" }));
  expect(row("London")).toBe("true");
  expect(row("Paris")).toBe("false");

  toBound(zoomOut);
  expect(zoomIn.getAttribute("aria-disabled")).not.toBe("true");
  expect(marker("London", true)).toBeTruthy();
  expect(marker("Paris", false)).toBeTruthy();
  expect(JSON.stringify(value)).toBe(original);
});

it("retires offscreen map links and restores their original destinations on zoom out", async () => {
  const value = document([
    {
      id: "map",
      type: "Map",
      props: {
        caption: "Source locations",
        markers: [
          { id: "london", label: "London", latitude: 51.5074, longitude: -0.1278 },
          { id: "paris", label: "Paris", latitude: 48.8566, longitude: 2.3522 },
        ],
      },
    },
  ]);
  const original = JSON.stringify(value);
  show(value);
  await act(async () => {});
  fireEvent.click(screen.getByRole("button", { name: "Load external content: Source locations" }));
  const paris = screen.getByRole("link", { name: "Paris" });
  const href = paris.getAttribute("href");
  const zoomIn = screen.getByRole("button", { name: "Zoom in" });
  const zoomOut = screen.getByRole("button", { name: "Zoom out" });
  for (let step = 0; step < 14; step++) fireEvent.click(zoomIn);

  expect(paris.isConnected).toBe(false);
  expect(screen.queryByRole("link", { name: "Paris" })).toBeNull();
  const places = screen.getByRole("table", { name: "Locations in Source locations" });
  expect(within(places).getByRole("rowheader", { name: "Paris" })).toBeTruthy();

  for (let step = 0; step < 14; step++) fireEvent.click(zoomOut);

  const restored = screen.getByRole("link", { name: "Paris" });
  expect(restored.getAttribute("href")).toBe(href);
  expect(restored.getAttribute("target")).toBe("_blank");
  expect(restored.getAttribute("rel")).toBe("noreferrer noopener");
  expect(JSON.stringify(value)).toBe(original);
});

it("keeps image loading and failures explicit and retries the original URL on user request", async () => {
  show(
    document([
      {
        id: "image",
        type: "Image",
        props: {
          src: "https://example.test/review.png",
          alt: "Review diagram",
          caption: "Recorded illustration",
        },
      },
    ]),
  );
  expect(screen.queryByRole("img")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Load external content: Review diagram" }));
  const image = screen.getByRole("img", { name: "Review diagram" });
  expect(image.closest("figure")?.getAttribute("aria-busy")).toBe("true");
  fireEvent.error(image);
  expect(image.isConnected).toBe(false);
  expect(screen.getByText("This image is unavailable.")).toBeTruthy();
  expect(screen.getByText("Recorded illustration")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Retry" }));
  const retried = screen.getByRole("img", { name: "Review diagram" });
  expect(retried.getAttribute("src")).toBe("https://example.test/review.png");
  fireEvent.load(retried);
  expect(retried.closest("figure")?.getAttribute("aria-busy")).toBe("false");
  expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
  const trigger = screen.getByRole("button", { name: "View image: Review diagram" });
  act(() => trigger.focus());
  fireEvent.click(trigger);
  const preview = await screen.findByRole("dialog", { name: "Review diagram" });
  expect(within(preview).getByRole("img").getAttribute("src")).toBe(
    "https://example.test/review.png",
  );
  fireEvent.keyDown(preview, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(window.document.activeElement).toBe(trigger));
});

it.each(["Image", "Markdown"] as const)(
  "retains the %s reading context while its focused Retry is replaced",
  async (kind) => {
    show(
      document([
        kind === "Image"
          ? {
              id: "diagram",
              type: "Image",
              props: { src: "https://example.test/review.png", alt: "Review diagram" },
            }
          : {
              id: "diagram",
              type: "Markdown",
              props: { text: "![Review diagram](https://example.test/review.png)" },
            },
      ]),
    );
    await act(async () => {});
    fireEvent.click(screen.getByRole("button", { name: "Load external content: Review diagram" }));
    fireEvent.error(screen.getByRole("img", { name: "Review diagram" }));
    const retry = screen.getByRole("button", { name: "Retry" });
    act(() => retry.focus());
    fireEvent.click(retry);
    const host = window.document.activeElement as HTMLElement;
    expect(host.getAttribute("aria-label")).toBe("Review diagram");
    expect(host.tabIndex).toBe(-1);
    expect(host.contains(screen.getByRole("button", { name: "View image: Review diagram" }))).toBe(
      true,
    );
    const retried = screen.getByRole("img", { name: "Review diagram" });
    expect(retried.getAttribute("src")).toBe("https://example.test/review.png");
    fireEvent.load(retried);
    expect(window.document.activeElement).toBe(host);
    fireEvent.error(retried);
    expect(window.document.activeElement).toBe(host);
    expect(host.contains(screen.getByRole("button", { name: "Retry" }))).toBe(true);
  },
);

it.each(["load", "error"] as const)(
  "preserves later editing focus when a retried image settles with %s",
  async (result) => {
    show(
      document(
        [
          {
            id: "diagram",
            type: "Image",
            props: { src: "https://example.test/review.png", alt: "Review diagram" },
          },
          { id: "note", type: "Input", props: { field: "note", label: "Review note" } },
        ],
        [{ id: "note", value: "Keep this draft." }],
      ),
    );
    await act(async () => {});
    fireEvent.click(screen.getByRole("button", { name: "Load external content: Review diagram" }));
    fireEvent.error(screen.getByRole("img", { name: "Review diagram" }));
    const retry = screen.getByRole("button", { name: "Retry" });
    act(() => retry.focus());
    fireEvent.click(retry);
    const note = screen.getByRole("textbox", { name: "Review note" });
    act(() => note.focus());
    const retried = screen.getByRole("img", { name: "Review diagram" });
    fireEvent[result](retried);
    expect(window.document.activeElement).toBe(note);
    expect(note).toHaveProperty("value", "Keep this draft.");
    if (result === "load") fireEvent.error(retried);
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(window.document.activeElement).toBe(note);
  },
);

it("validates date bounds through the document owner before any form action", async () => {
  show(GENUI_REPORT);
  const date = screen.getByLabelText("Review date");
  await waitFor(() => expect((date as HTMLInputElement).value).toBe("2026-11-10"));
  fireEvent.change(date, { target: { value: "2026-10-01" } });
  expect(screen.getByText("Date must be on or after 2026-10-27.")).toBeTruthy();
  await act(async () => {
    fireEvent.submit(screen.getByRole("form", { name: "Delivery scenario" }));
  });
  expect(screen.getByText("No scenario has been applied.")).toBeTruthy();
  fireEvent.change(date, { target: { value: "2028-01-01" } });
  expect(screen.getByText("Date must be on or before 2027-12-31.")).toBeTruthy();
});

it("returns an invalid report submission to its first field without losing the draft", async () => {
  show(GENUI_REPORT);
  const team = screen.getByRole("textbox", { name: "Delivery team" });
  const weeks = screen.getByRole("textbox", { name: "Delivery weeks" });
  const form = screen.getByRole("form", { name: "Delivery scenario" });
  const submit = screen.getByRole("button", { name: "Apply scenario" });
  await act(async () => {});
  fireEvent.change(team, { target: { value: "" } });
  fireEvent.change(weeks, { target: { value: "-" } });
  submit.focus();
  await act(async () => {
    fireEvent.submit(form);
  });
  expect(window.document.activeElement).toBe(team);
  expect((weeks as HTMLInputElement).value).toBe("-");
  expect(screen.getByText("No scenario has been applied.")).toBeTruthy();

  fireEvent.change(team, { target: { value: "Review team" } });
  submit.focus();
  await act(async () => {
    fireEvent.submit(form);
  });
  expect(window.document.activeElement).toBe(weeks);
  fireEvent.change(weeks, { target: { value: "6" } });
  submit.focus();
  await act(async () => {
    fireEvent.submit(form);
  });
  expect(window.document.activeElement).toBe(submit);
  expect(screen.getByText("Applied scenario for Review team: 6 delivery weeks.")).toBeTruthy();
  expect(screen.getByText("Action accepted.").getAttribute("role")).toBe("status");
});

it.each(["Input", "TextArea", "Select", "MultiSelect", "RadioGroup", "DatePicker"] as const)(
  "connects the %s validation to its control and focuses it on submission",
  async (type) => {
    const choice = ["Select", "MultiSelect", "RadioGroup"].includes(type);
    const field: UiNode = {
      id: "requiredField",
      type,
      props: {
        field: "draft",
        label: "Required detail",
        ...(type === "DatePicker" ? { min: "2026-10-27" } : { rules: { required: true } }),
        ...(choice ? { options: [{ value: "first", label: "First option" }] } : {}),
      },
    };
    show(
      document(
        [
          {
            id: "form",
            type: "Form",
            props: { action: "save", submitLabel: "Save local" },
            children: [field],
          },
          { id: "output", type: "Text", props: { text: { state: "saved" } } },
        ],
        [
          {
            id: "draft",
            value: type === "DatePicker" ? "2026-01-01" : type === "MultiSelect" ? [] : "",
          },
          { id: "saved", value: "Not saved" },
        ],
        [
          {
            id: "save",
            steps: [{ type: "set", state: "saved", value: { literal: "Saved locally" } }],
          },
        ],
      ),
    );
    await act(async () => {});
    const target =
      type === "Select"
        ? screen.getByRole("combobox", { name: "Required detail" })
        : type === "MultiSelect" || type === "RadioGroup"
          ? screen.getByRole(type === "MultiSelect" ? "checkbox" : "radio", {
              name: "First option",
            })
          : screen.getByLabelText("Required detail");
    const validation =
      type === "MultiSelect" || type === "RadioGroup"
        ? screen.getByRole(type === "MultiSelect" ? "group" : "radiogroup", {
            name: "Required detail",
          })
        : target;
    expect(validation.getAttribute("aria-invalid")).toBe("true");
    const description = window.document.getElementById(
      validation.getAttribute("aria-describedby") ?? "",
    );
    expect(description?.textContent).toBe(
      type === "DatePicker" ? "Date must be on or after 2026-10-27." : "This field is required.",
    );
    const submit = screen.getByRole("button", { name: "Save local" });
    submit.focus();
    await act(async () => {
      fireEvent.submit(screen.getByRole("form", { name: "Save local" }));
    });
    expect(window.document.activeElement).toBe(target);
    expect(screen.getByText("Not saved")).toBeTruthy();
  },
);

it("keeps invalid-field focus inside the submitted form and its visible view", async () => {
  show(
    document(
      [
        {
          id: "outside",
          type: "Input",
          props: { field: "outside", label: "Other draft", rules: { required: true } },
        },
        {
          id: "form",
          type: "Form",
          props: { action: "save", submitLabel: "Save local" },
          children: [
            {
              id: "tabs",
              type: "Tabs",
              props: {
                field: "tab",
                items: [
                  { value: "hidden", label: "Other view" },
                  { value: "current", label: "Current view" },
                ],
              },
              children: [
                {
                  id: "hidden",
                  type: "Input",
                  props: { field: "hidden", label: "Hidden draft", rules: { required: true } },
                },
                {
                  id: "current",
                  type: "Input",
                  props: { field: "current", label: "Current draft", rules: { required: true } },
                },
              ],
            },
          ],
        },
        { id: "output", type: "Text", props: { text: { state: "saved" } } },
      ],
      [
        { id: "outside", value: "" },
        { id: "hidden", value: "" },
        { id: "current", value: "" },
        { id: "tab", value: "current" },
        { id: "saved", value: "Not saved" },
      ],
      [
        {
          id: "save",
          steps: [{ type: "set", state: "saved", value: { literal: "Saved locally" } }],
        },
      ],
    ),
  );
  await act(async () => {});
  const submit = screen.getByRole("button", { name: "Save local" });
  submit.focus();
  await act(async () => {
    fireEvent.submit(screen.getByRole("form", { name: "Save local" }));
  });
  const current = screen.getByRole("textbox", { name: "Current draft" });
  expect(window.document.activeElement).toBe(current);
  fireEvent.change(current, { target: { value: "Ready" } });
  submit.focus();
  await act(async () => {
    fireEvent.submit(screen.getByRole("form", { name: "Save local" }));
  });
  expect(window.document.activeElement).toBe(submit);
  expect(screen.getByText("Saved locally")).toBeTruthy();
});

it("validates visible form fields and resets declared fields even while their conditional view is absent", async () => {
  show(
    document(
      [
        {
          id: "form",
          type: "Form",
          props: { action: "save", submitLabel: "Save local", resetLabel: "Restore defaults" },
          children: [
            { id: "toggle", type: "Checkbox", props: { field: "show", label: "Show details" } },
            {
              id: "conditional",
              type: "If",
              props: { when: { state: "show" } },
              children: [
                {
                  id: "detail",
                  type: "Input",
                  props: { field: "detail", label: "Required detail", rules: { required: true } },
                },
              ],
            },
          ],
        },
        { id: "output", type: "Text", props: { text: { state: "saved" } } },
        {
          id: "draft",
          type: "Text",
          props: {
            text: {
              call: "concat",
              args: [{ array: [{ literal: "Draft: " }, { state: "detail" }] }],
            },
          },
        },
      ],
      [
        { id: "show", value: false },
        { id: "detail", value: "" },
        { id: "saved", value: "Not saved" },
      ],
      [
        {
          id: "save",
          steps: [{ type: "set", state: "saved", value: { literal: "Saved locally" } }],
        },
      ],
    ),
  );
  await act(async () => {});
  await act(async () => {
    fireEvent.submit(screen.getByRole("form", { name: "Save local" }));
  });
  await screen.findByText("Saved locally");
  fireEvent.click(screen.getByLabelText("Show details", { selector: "input" }));
  expect(screen.getByText("This field is required.")).toBeTruthy();
  fireEvent.change(screen.getByRole("textbox", { name: "Required detail" }), {
    target: { value: "Changed" },
  });
  expect(screen.getByText("Draft: Changed")).toBeTruthy();
  fireEvent.click(screen.getByLabelText("Show details", { selector: "input" }));
  expect(screen.queryByRole("textbox", { name: "Required detail" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Restore defaults" }));
  expect(screen.getByText("Draft:", { exact: true })).toBeTruthy();
});

it.each([
  { label: "malformed JSON", result: "{not a document", status: "ok" as const },
  {
    label: "an undeclared property",
    result: JSON.stringify({ ...report, onAction: "send" }),
    status: "ok" as const,
  },
  { label: "a failed tool", result: JSON.stringify(report), status: "err" as const },
  { label: "a denied tool", result: JSON.stringify(report), status: "denied" as const },
  { label: "an unfinished tool", result: JSON.stringify(report), status: "running" as const },
])(
  "preserves the generic inspector for $label instead of rendering arguments or partial success",
  ({ result, status }) => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    testClient.render(
      <ComponentBoundary
        scope="GenUI captured result"
        fallback={<pre aria-label="Captured source">{result}</pre>}
      >
        <GenUIResult
          context={capturedContext(result, "en", { status, args: JSON.stringify(report) })}
        />
      </ComponentBoundary>,
    );
    expect(screen.getByRole("alert")).toBeTruthy();
    expect(screen.getByLabelText("Captured source").textContent).toBe(result);
    expect(screen.queryByRole("region", { name: report.title })).toBeNull();
  },
);

const releaseModules = import.meta.glob<{ GenUIResult: PluginComponent }>(
  "../../../../plugins/genui/package/io.github.tangerg.flame/components/component.js",
  { eager: true },
);
const releasedComponent = Object.values(releaseModules)[0]?.GenUIResult;
if (!releasedComponent) throw new Error("genui test requires its built release component");

it.each([
  ["authored component", GenUIResult],
  ["published ESM", releasedComponent],
] as const)("passes a trusted %s failure through OpenUI to the host inspector", (_name, Entry) => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  const result = JSON.stringify(report);
  const capturedReport = (locale: string) => (
    <ComponentBoundary
      scope="GenUI captured result"
      fallback={<pre aria-label="Captured source">{result}</pre>}
    >
      <Entry context={capturedContext(result, locale)} />
    </ComponentBoundary>
  );
  const rendered = testClient.render(capturedReport("en"));
  expect(screen.getByRole("region", { name: report.title })).toBeTruthy();
  vi.spyOn(ui, "Stats").mockImplementation(() => {
    throw new Error("recorded metric rendering failed");
  });
  rendered.rerender(capturedReport("zh"));
  expect(screen.getByRole("alert")).toBeTruthy();
  expect(screen.getByLabelText("Captured source").textContent).toBe(result);
  expect(screen.queryByRole("region", { name: report.title })).toBeNull();
});

it("renders the recorded first frame on the server without component context I/O", () => {
  const html = renderToStaticMarkup(
    <ApplicationCalloutLabels>
      <GenUIResult context={capturedContext(JSON.stringify(GENUI_REPORT))} />
    </ApplicationCalloutLabels>,
  );
  expect(html).toContain("Agent plugin delivery review");
  expect(html).toContain("9007199254740993");
  expect(html).toContain('value="4"');
  expect(html).toContain(
    '<caption style="text-align:start">Plugin delivery responsibilities</caption>',
  );
  expect(html).not.toContain("<script");
  expect(Presentation.read(JSON.stringify(GENUI_REPORT)).snapshot()).toEqual(GENUI_REPORT);
});

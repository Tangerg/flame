import { unavailableComponentNotifications } from "@/test/unavailableNotifications";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { mockSelectLayout, mockSelectViewport } from "@/test/selectLayout";
let testClient: TestClient;
prepareTestClient(() => {
  mockSelectLayout();
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { type ComponentContext } from "@flame/component-sdk";
import { createFlameClient, type FlameClient } from "@flame/runtime-client";
import { Schedules } from "../../../../plugins/schedules/component/component";
import { RUNTIME_AGENT_SESSION_SNAPSHOTS } from "./agentSessionSnapshots";
import { VISUAL_CAPABILITIES, VISUAL_WORKSPACE_PATH } from "./agentFixtureFacts";
import { VISUAL_SCHEDULES_BINDING, VISUAL_SCHEDULES_PAGE } from "./componentSnapshots";
import { createVisualRuntimeTransport } from "./createVisualRuntimeTransport";
import { REVIEW_DIFF } from "./diffSnapshot";
const releases: Array<() => Promise<void>> = [];

afterEach(async () => {
  cleanup();
  await Promise.all(releases.splice(0).map((release) => release()));
  vi.restoreAllMocks();
});

async function mountSchedules() {
  const owner = new AbortController();
  const runtime = createFlameClient(
    createVisualRuntimeTransport(
      structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
      VISUAL_CAPABILITIES,
      async () => REVIEW_DIFF,
    ),
    { capabilities: () => VISUAL_CAPABILITIES },
  );
  const call = vi.spyOn(runtime.plugins, "call");
  let context: ComponentContext = {
    component: { ...VISUAL_SCHEDULES_BINDING, componentId: "schedules" },
    binding: VISUAL_SCHEDULES_BINDING,
    runtime,
    workspace: { ref: { path: VISUAL_WORKSPACE_PATH } },
    telemetry: { read: () => ({ spans: [], metrics: [], logs: [] }), clear() {} },
    locale: "en",
    signal: owner.signal,
    track: (operation) => operation(owner.signal),
    saveFile: async () => "requested",
    notifications: unavailableComponentNotifications,
    subscribeRootRunSettlements() {
      throw new Error("This fixture must not observe run settlements");
    },
    registerCommand() {
      throw new Error("This fixture must not register client commands");
    },
    compose: () => null,
  };
  releases.push(async () => {
    owner.abort();
    await runtime.close();
  });
  const rendered = testClient.render(
    <div data-testid="viewport" style={{ overflowY: "auto", height: 400 }}>
      <Schedules context={context} />
    </div>,
  );
  mockSelectViewport(screen.getByTestId("viewport"));
  await screen.findByRole("heading", { name: VISUAL_SCHEDULES_PAGE.Rows[0]!.Title });
  await waitFor(() => expect(screen.queryByText("Reading schedules…")).toBeNull());
  function changeContext(
    changes: Partial<Pick<ComponentContext, "locale" | "runtime" | "track" | "signal">>,
  ) {
    context = Object.freeze({ ...context, ...changes });
    rendered.rerender(
      <div data-testid="viewport" style={{ overflowY: "auto", height: 400 }}>
        <Schedules context={context} />
      </div>,
    );
  }
  return { call, runtime, context, changeContext };
}

function holdModelReads(runtime: FlameClient) {
  const original = runtime.providers.list.bind(runtime.providers);
  const catalogue = vi.spyOn(runtime.providers, "list");
  function hold() {
    let release!: (refusal?: Error) => void;
    const gate = new Promise<Error | undefined>((resolve) => (release = resolve));
    catalogue.mockImplementationOnce(async (...args) => {
      const refusal = await gate;
      if (refusal) throw refusal;
      return original(...args);
    });
    return release;
  }
  return { catalogue, hold };
}

async function chooseModel(editor: HTMLElement, name: string) {
  const model = within(editor).getByRole("combobox", { name: "Model" });
  fireEvent.click(model);
  const option = await screen.findByRole("option", { name });
  fireEvent.pointerDown(option, { pointerType: "mouse" });
  fireEvent.click(option);
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  return model;
}

async function openSavedSchedule() {
  fireEvent.click(
    screen.getByRole("button", { name: "More actions for Nightly dependency audit" }),
  );
  fireEvent.click(await screen.findByRole("menuitem", { name: "Edit" }));
  const editor = await screen.findByRole("dialog", { name: "Edit Nightly dependency audit" });
  await within(editor).findByRole("button", { name: "Refresh model choices" });
  return editor;
}

it("starts a changed page query at its first result without moving search focus or reading again", async () => {
  const { call } = await mountSchedules();
  const query = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this page" });
  const viewport = screen.getByTestId("viewport");
  const issued = call.mock.calls.length;
  query.focus();
  viewport.scrollTop = 280;

  fireEvent.change(query, { target: { value: "nightly" } });

  expect(viewport.scrollTop).toBe(0);
  expect(screen.getByRole("heading", { name: VISUAL_SCHEDULES_PAGE.Rows[0]!.Title })).toBeTruthy();
  expect(screen.queryByRole("heading", { name: VISUAL_SCHEDULES_PAGE.Rows[1]!.Title })).toBeNull();
  expect(document.activeElement).toBe(query);
  viewport.scrollTop = 125;
  fireEvent.change(query, { target: { value: "nightly" } });
  expect(viewport.scrollTop).toBe(125);
  expect(call.mock.calls.length).toBe(issued);
});

it.each(["typing", "button", "Escape"] as const)(
  "reveals the page beginning when clearing search with %s",
  async (method) => {
    const { call } = await mountSchedules();
    const query = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this page" });
    fireEvent.change(query, { target: { value: "nightly" } });
    query.focus();
    const viewport = screen.getByTestId("viewport");
    viewport.scrollTop = 125;
    const issued = call.mock.calls.length;

    let unhandledEscape = false;
    if (method === "typing") fireEvent.change(query, { target: { value: "" } });
    else if (method === "button")
      fireEvent.click(screen.getByRole("button", { name: "Clear search" }));
    else unhandledEscape = fireEvent.keyDown(query, { key: "Escape" });

    expect(unhandledEscape).toBe(false);
    expect(viewport.scrollTop).toBe(0);
    expect(query.value).toBe("");
    expect(
      screen.getByRole("heading", { name: VISUAL_SCHEDULES_PAGE.Rows[0]!.Title }),
    ).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: VISUAL_SCHEDULES_PAGE.Rows[1]!.Title }),
    ).toBeTruthy();
    expect(document.activeElement).toBe(query);
    expect(call.mock.calls.length).toBe(issued);
  },
);

it("starts a changed status filter at its first result and retains the query", async () => {
  const { call } = await mountSchedules();
  const query = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this page" });
  fireEvent.change(query, { target: { value: "workspace" } });
  const status = screen.getByRole("combobox", { name: "Schedule status" });
  status.focus();
  fireEvent.click(status);
  const option = await screen.findByRole("option", { name: "Enabled" });
  const viewport = screen.getByTestId("viewport");
  viewport.scrollTop = 280;
  const issued = call.mock.calls.length;

  fireEvent.pointerDown(option, { pointerType: "mouse" });
  fireEvent.click(option);

  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  expect(viewport.scrollTop).toBe(0);
  expect(query.value).toBe("workspace");
  expect(status.textContent).toContain("Enabled");
  expect(document.activeElement).toBe(status);
  expect(screen.getByRole("heading", { name: VISUAL_SCHEDULES_PAGE.Rows[0]!.Title })).toBeTruthy();
  expect(screen.queryByRole("heading", { name: VISUAL_SCHEDULES_PAGE.Rows[1]!.Title })).toBeNull();
  expect(call.mock.calls.length).toBe(issued);
});

it("opens a bounded editor and returns to the retained catalog on Escape", async () => {
  await mountSchedules();
  const query = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this page" });
  fireEvent.change(query, { target: { value: "nightly" } });
  const viewport = screen.getByTestId("viewport");
  viewport.scrollTop = 125;
  const add = screen.getByRole("button", { name: "Add schedule" });
  add.focus();
  fireEvent.click(add);
  const editor = await screen.findByRole("dialog", { name: "New schedule" });
  const title = within(editor).getByRole<HTMLInputElement>("textbox", { name: "Title" });
  await waitFor(() => expect(document.activeElement).toBe(title));
  fireEvent.change(title, { target: { value: "Retained task context" } });
  expect(within(editor).getByRole("button", { name: "Cancel" })).toBeTruthy();
  expect(within(editor).getByRole("button", { name: "Save schedule" })).toBeTruthy();
  fireEvent.keyDown(title, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(add));
  expect(query.value).toBe("nightly");
  expect(viewport.scrollTop).toBe(125);
  expect(screen.queryByRole("heading", { name: "Weekly maintenance notes" })).toBeNull();
});

it("clears a filtered empty page back to its records and search without another Runtime call", async () => {
  const { call } = await mountSchedules();
  const query = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this page" });
  const status = screen.getByRole("combobox", { name: "Schedule status" });
  fireEvent.click(status);
  const option = await screen.findByRole("option", { name: "Disabled" });
  fireEvent.pointerDown(option, { pointerType: "mouse" });
  fireEvent.click(option);
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  expect(status.textContent).toContain("Disabled");
  fireEvent.change(query, { target: { value: "release notes" } });
  expect(screen.getByText("No matching schedules on this page.")).toBeTruthy();
  expect(screen.queryByRole("heading", { name: VISUAL_SCHEDULES_PAGE.Rows[0]!.Title })).toBeNull();
  expect(screen.queryByRole("heading", { name: VISUAL_SCHEDULES_PAGE.Rows[1]!.Title })).toBeNull();

  const issued = call.mock.calls.length;
  const clear = screen.getByRole("button", { name: "Clear filters" });
  clear.focus();
  const viewport = screen.getByTestId("viewport");
  viewport.scrollTop = 125;
  fireEvent.click(clear);
  expect(viewport.scrollTop).toBe(0);
  expect(query.value).toBe("");
  expect(status.textContent).toContain("All statuses");
  expect(screen.getByRole("heading", { name: VISUAL_SCHEDULES_PAGE.Rows[0]!.Title })).toBeTruthy();
  expect(screen.getByRole("heading", { name: VISUAL_SCHEDULES_PAGE.Rows[1]!.Title })).toBeTruthy();
  expect(screen.queryByText("No matching schedules on this page.")).toBeNull();
  expect(screen.getByRole("searchbox", { name: "Search this page" })).toBe(query);
  expect(document.activeElement).toBe(query);
  expect(call.mock.calls.length).toBe(issued);
});

it("cancels a bounded deletion decision back to its record without issuing a command", async () => {
  const { call } = await mountSchedules();
  const trigger = screen.getByRole("button", { name: "More actions for Nightly dependency audit" });
  trigger.focus();
  fireEvent.click(trigger);
  fireEvent.click(await screen.findByRole("menuitem", { name: "Delete" }));
  const confirmation = await screen.findByRole("alertdialog", {
    name: "Delete Nightly dependency audit?",
  });
  const cancel = within(confirmation).getByRole("button", { name: "Cancel" });
  await waitFor(() => expect(document.activeElement).toBe(cancel));
  fireEvent.keyDown(cancel, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(trigger));
  expect(call.mock.calls.some(([request]) => request.method === "flame.schedules/delete.v1")).toBe(
    false,
  );
});

it.each(["Cancel", "Close schedule editor"])("%s returns to the retained catalog", async (name) => {
  await mountSchedules();
  const add = screen.getByRole("button", { name: "Add schedule" });
  add.focus();
  fireEvent.click(add);
  const editor = await screen.findByRole("dialog", { name: "New schedule" });
  fireEvent.click(within(editor).getByRole("button", { name }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(add));
});

it("closes model choices before allowing Escape to cancel the editor", async () => {
  await mountSchedules();
  const add = screen.getByRole("button", { name: "Add schedule" });
  add.focus();
  fireEvent.click(add);
  const editor = await screen.findByRole("dialog", { name: "New schedule" });
  const model = within(editor).getByRole("combobox", { name: "Model" });
  fireEvent.click(model);
  const option = await screen.findByRole("option", { name: "Runtime default model" });
  fireEvent.keyDown(option, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  expect(screen.getByRole("dialog", { name: "New schedule" })).toBe(editor);
  await waitFor(() => expect(document.activeElement).toBe(model));
  fireEvent.keyDown(model, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(add));
});

it("keeps a template draft and its focused model refresh through held catalogue reads", async () => {
  const { call, runtime } = await mountSchedules();
  const { catalogue, hold } = holdModelReads(runtime);
  const templates = screen.getByRole("button", { name: "Schedule templates" });
  templates.focus();
  fireEvent.click(templates);
  fireEvent.click(await screen.findByRole("menuitem", { name: "Hourly progress check" }));
  const editor = await screen.findByRole("dialog", { name: "New schedule" });
  const fields = ["Title", "Instructions", "Cron expression", "Time zone", "Workspace path"].map(
    (name) => within(editor).getByRole<HTMLInputElement>("textbox", { name }),
  );
  const refresh = await within(editor).findByRole("button", { name: "Refresh model choices" });
  const model = await chooseModel(editor, "openai / GPT-5.6 Sol");
  const draft = fields.map((field) => field.value);
  expect(draft.slice(0, 3)).toEqual([
    "Hourly progress check",
    "Inspect the current workspace and summarize recent progress, outstanding work and any blockers.",
    "0 * * * *",
  ]);
  const expectRetained = () => {
    expect(fields.map((field) => field.value)).toEqual(draft);
    expect(model.textContent).toContain("openai / GPT-5.6 Sol");
  };

  const refuse = hold();
  refresh.focus();
  fireEvent.click(refresh);
  expect(within(editor).getByRole("button", { name: "Refreshing model choices…" })).toBe(refresh);
  expect(refresh.getAttribute("aria-busy")).toBe("true");
  expect(refresh.getAttribute("aria-disabled")).toBe("true");
  expect(refresh.hasAttribute("disabled")).toBe(false);
  expect(document.activeElement).toBe(refresh);
  fireEvent.click(refresh);
  expect(catalogue).toHaveBeenCalledTimes(1);

  refuse(new Error("catalogue unavailable"));
  expect(await within(editor).findByText(/Model choices could not be read/)).toBeTruthy();
  expect(within(editor).getByRole("button", { name: "Refresh model choices" })).toBe(refresh);
  expect(document.activeElement).toBe(refresh);
  expectRetained();

  const accept = hold();
  fireEvent.click(refresh);
  expect(refresh.getAttribute("aria-busy")).toBe("true");
  accept();
  await waitFor(() =>
    expect(within(editor).getByRole("button", { name: "Refresh model choices" })).toBe(refresh),
  );
  expect(within(editor).queryByText(/Model choices could not be read/)).toBeNull();
  expect(document.activeElement).toBe(refresh);
  expect(catalogue).toHaveBeenCalledTimes(2);
  expectRetained();

  fireEvent.click(within(editor).getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(templates));
  expect(call.mock.calls.every(([request]) => request.method === "flame.schedules/list.v1")).toBe(
    true,
  );
});

it("preserves later draft focus through a model read and ignores a read settling after Cancel", async () => {
  const { runtime } = await mountSchedules();
  const { hold } = holdModelReads(runtime);
  const modelCatalogue = vi.spyOn(runtime.models, "list");
  const add = screen.getByRole("button", { name: "Add schedule" });
  add.focus();
  fireEvent.click(add);
  const editor = await screen.findByRole("dialog", { name: "New schedule" });
  const refresh = await within(editor).findByRole("button", { name: "Refresh model choices" });
  const model = await chooseModel(editor, "openai / GPT-5.6 Sol");
  const instructions = within(editor).getByRole<HTMLTextAreaElement>("textbox", {
    name: "Instructions",
  });

  const release = hold();
  refresh.focus();
  fireEvent.click(refresh);
  instructions.focus();
  fireEvent.change(instructions, { target: { value: "Summarize the release checklist." } });
  release();
  await waitFor(() =>
    expect(within(editor).getByRole("button", { name: "Refresh model choices" })).toBe(refresh),
  );
  expect(document.activeElement).toBe(instructions);
  expect(instructions.value).toBe("Summarize the release checklist.");
  expect(model.textContent).toContain("openai / GPT-5.6 Sol");

  const late = hold();
  fireEvent.click(refresh);
  expect(refresh.getAttribute("aria-busy")).toBe("true");
  fireEvent.click(within(editor).getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(add));
  late();
  await waitFor(() => expect(modelCatalogue).toHaveBeenCalledTimes(2));
  await act(async () => {
    await modelCatalogue.mock.results[1]!.value;
  });
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(document.activeElement).toBe(add);
});

it("returns an idle edit to its available record opener", async () => {
  await mountSchedules();
  const trigger = screen.getByRole("button", { name: "More actions for Nightly dependency audit" });
  trigger.focus();
  fireEvent.click(trigger);
  fireEvent.click(await screen.findByRole("menuitem", { name: "Edit" }));
  const editor = await screen.findByRole("dialog", { name: "Edit Nightly dependency audit" });
  fireEvent.click(within(editor).getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(trigger));
});

it("retains accepted model choices, a saved selection and later draft focus across a display-locale change", async () => {
  const { runtime, context, changeContext } = await mountSchedules();
  const editor = await openSavedSchedule();
  const model = within(editor).getByRole("combobox", { name: "Model" });
  await waitFor(() => expect(model.textContent).toContain("openai / GPT-5.6 Sol"));
  const instructions = within(editor).getByRole<HTMLTextAreaElement>("textbox", {
    name: "Instructions",
  });
  fireEvent.change(instructions, {
    target: { value: "Review the dependency report.\n保留模型草稿" },
  });
  const catalogue = vi.spyOn(runtime.providers, "list");
  const models = vi.spyOn(runtime.models, "list");
  catalogue.mockRejectedValueOnce(new Error("catalogue unavailable"));
  act(() => instructions.focus());
  await act(async () => changeContext({ locale: "zh" }));
  expect(catalogue).not.toHaveBeenCalled();
  expect(models).not.toHaveBeenCalled();
  expect(context.signal.aborted).toBe(false);
  expect(screen.getByRole("dialog", { name: "编辑 Nightly dependency audit" })).toBe(editor);
  expect(within(editor).getByRole("combobox", { name: "模型" })).toBe(model);
  expect(model.textContent).toContain("openai / GPT-5.6 Sol");
  expect(within(editor).getByRole("textbox", { name: "指令" })).toBe(instructions);
  expect(instructions.value).toBe("Review the dependency report.\n保留模型草稿");
  expect(document.activeElement).toBe(instructions);
  const refresh = within(editor).getByRole("button", { name: "刷新模型选项" });
  fireEvent.click(refresh);
  await within(editor).findByText("无法读取模型选项：catalogue unavailable");
  expect(catalogue).toHaveBeenCalledTimes(1);
  expect(models).not.toHaveBeenCalled();
  expect(model.textContent).toContain("openai / GPT-5.6 Sol");
  expect(instructions.value).toBe("Review the dependency report.\n保留模型草稿");
  expect(document.activeElement).toBe(instructions);
  fireEvent.click(model);
  const selected = await screen.findByRole("option", { name: "openai / GPT-5.6 Sol" });
  expect(selected.getAttribute("aria-selected")).toBe("true");
  expect(screen.getByRole("option", { name: "Runtime 默认模型" })).toBeTruthy();
  fireEvent.keyDown(selected, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
});

it("accepts the original held model refresh after a display-locale change without replacing its signal", async () => {
  const { runtime, context, changeContext } = await mountSchedules();
  const editor = await openSavedSchedule();
  const model = within(editor).getByRole("combobox", { name: "Model" });
  await waitFor(() => expect(model.textContent).toContain("openai / GPT-5.6 Sol"));
  const { catalogue, hold } = holdModelReads(runtime);
  const models = vi.spyOn(runtime.models, "list");
  const release = hold();
  const refresh = within(editor).getByRole("button", { name: "Refresh model choices" });
  const instructions = within(editor).getByRole<HTMLTextAreaElement>("textbox", {
    name: "Instructions",
  });
  try {
    act(() => refresh.focus());
    fireEvent.click(refresh);
    await waitFor(() => expect(catalogue).toHaveBeenCalledTimes(1));
    const originalSignal = catalogue.mock.calls[0]![0]!;
    expect(originalSignal.aborted).toBe(false);
    fireEvent.change(instructions, { target: { value: "Keep the original model read. 保留" } });
    act(() => instructions.focus());
    await act(async () => changeContext({ locale: "zh" }));
    expect(catalogue).toHaveBeenCalledTimes(1);
    expect(models).not.toHaveBeenCalled();
    expect(originalSignal.aborted).toBe(false);
    expect(context.signal.aborted).toBe(false);
    expect(within(editor).getByRole("button", { name: "正在刷新模型选项…" })).toBe(refresh);
    expect(refresh.getAttribute("aria-busy")).toBe("true");
    expect(model.textContent).toContain("openai / GPT-5.6 Sol");
    expect(instructions.value).toBe("Keep the original model read. 保留");
    expect(document.activeElement).toBe(instructions);
    await act(async () => release());
    await within(editor).findByRole("button", { name: "刷新模型选项" });
    expect(refresh.getAttribute("aria-busy")).not.toBe("true");
    expect(catalogue).toHaveBeenCalledTimes(1);
    expect(models).toHaveBeenCalledTimes(1);
    expect(models).toHaveBeenLastCalledWith("openai", originalSignal);
    expect(originalSignal.aborted).toBe(false);
    expect(model.textContent).toContain("openai / GPT-5.6 Sol");
    expect(instructions.value).toBe("Keep the original model read. 保留");
    expect(document.activeElement).toBe(instructions);
  } finally {
    await act(async () => release());
  }
});

it("fences a replaced model owner while retaining the saved unavailable fallback and ignoring an old reply", async () => {
  const { runtime, changeContext } = await mountSchedules();
  const editor = await openSavedSchedule();
  const model = within(editor).getByRole("combobox", { name: "Model" });
  await waitFor(() => expect(model.textContent).toContain("openai / GPT-5.6 Sol"));
  const oldReply = Promise.withResolvers<Awaited<ReturnType<FlameClient["models"]["list"]>>>();
  const oldModels = vi.spyOn(runtime.models, "list").mockImplementationOnce(() => oldReply.promise);
  const owner = new AbortController();
  const nextRuntime = createFlameClient(
    createVisualRuntimeTransport(
      structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
      VISUAL_CAPABILITIES,
      async () => REVIEW_DIFF,
    ),
    { capabilities: () => VISUAL_CAPABILITIES },
  );
  releases.push(async () => {
    owner.abort();
    await nextRuntime.close();
  });
  const nextReply = Promise.withResolvers<void>();
  const originalProviders = nextRuntime.providers.list.bind(nextRuntime.providers);
  const nextProviders = vi
    .spyOn(nextRuntime.providers, "list")
    .mockImplementationOnce(async (...args) => {
      await nextReply.promise;
      return originalProviders(...args);
    });
  const nextModels = vi.spyOn(nextRuntime.models, "list");
  try {
    fireEvent.click(within(editor).getByRole("button", { name: "Refresh model choices" }));
    await waitFor(() => expect(oldModels).toHaveBeenCalledTimes(1));
    const retiredSignal = oldModels.mock.calls[0]![1]!;
    await act(async () =>
      changeContext({
        runtime: nextRuntime,
        signal: owner.signal,
        track: (operation) => operation(owner.signal),
      }),
    );
    expect(retiredSignal.aborted).toBe(true);
    expect(nextProviders).toHaveBeenCalledTimes(1);
    expect(nextModels).not.toHaveBeenCalled();
    expect(model.textContent).toContain("openai / gpt-5.6-sol (unavailable)");
    fireEvent.click(model);
    const fallback = await screen.findByRole("option", {
      name: "openai / gpt-5.6-sol (unavailable)",
    });
    expect(fallback.getAttribute("aria-selected")).toBe("true");
    expect(screen.queryByRole("option", { name: "openai / GPT-5.6 Sol" })).toBeNull();
    fireEvent.keyDown(fallback, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
    await act(async () => nextReply.resolve());
    await waitFor(() => expect(model.textContent).toContain("openai / GPT-5.6 Sol"));
    expect(nextModels).toHaveBeenCalledTimes(1);
    await act(async () => oldReply.resolve({ data: [] }));
    expect(model.textContent).toContain("openai / GPT-5.6 Sol");
    expect(nextProviders).toHaveBeenCalledTimes(1);
    expect(nextModels).toHaveBeenCalledTimes(1);
    expect(oldModels).toHaveBeenCalledTimes(1);
    expect(owner.signal.aborted).toBe(false);
  } finally {
    await act(async () => {
      oldReply.resolve({ data: [] });
      nextReply.resolve();
    });
  }
});

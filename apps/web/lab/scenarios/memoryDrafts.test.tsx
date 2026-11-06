import { unavailableComponentNotifications } from "@/test/unavailableNotifications";
import { mutationResult } from "@/test/runtimeClient";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { mockSelectLayout } from "@/test/selectLayout";
let testClient: TestClient;
prepareTestClient(() => {
  mockSelectLayout();
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi, type Mock } from "vitest";
import { createFlameClient, MutationLookupError } from "@flame/runtime-client";

import { type CallPluginRequest, type CallPluginResponse } from "@flame/runtime-client/wire";
import { type ComponentContext } from "@flame/component-sdk";
import { Memory } from "../../../../plugins/memory/component/component";
import { type MethodOutputs as MemoryOutputs } from "../../../../plugins/memory/package/io.github.tangerg.flame/contracts/methods.generated";
import { RUNTIME_AGENT_SESSION_SNAPSHOTS } from "./agentSessionSnapshots";
import { VISUAL_CAPABILITIES } from "./agentFixtureFacts";
import { VISUAL_MEMORY_BINDING, VISUAL_MEMORY_PAGE } from "./componentSnapshots";
import { createVisualRuntimeTransport } from "./createVisualRuntimeTransport";
import { REVIEW_DIFF } from "./diffSnapshot";
const releases: Array<() => Promise<void>> = [];

afterEach(async () => {
  cleanup();
  await Promise.all(releases.splice(0).map((release) => release()));
  vi.restoreAllMocks();
});

async function openMemory(
  list?: (request: CallPluginRequest) => Promise<CallPluginResponse>,
  locale = "en",
) {
  const owner = new AbortController();
  const transport = createVisualRuntimeTransport(
    structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
    VISUAL_CAPABILITIES,
    async () => REVIEW_DIFF,
  );
  const runtime = createFlameClient(transport, { capabilities: () => VISUAL_CAPABILITIES });
  releases.push(async () => {
    owner.abort();
    await runtime.close();
  });
  const settle = vi.fn<(request: CallPluginRequest, key: string) => Promise<CallPluginResponse>>(
    async (request) => ({
      output: JSON.stringify(
        request.method === "flame.memory/add.v2"
          ? ({
              Item: {
                ...VISUAL_MEMORY_PAGE.Rows[1]!,
                ID: "mem_00000000000000000000000000000003",
                Content: "Keep my unsaved memory draft.",
              },
              Changed: true,
            } satisfies MemoryOutputs["flame.memory/add.v2"])
          : request.method === "flame.memory/update.v2"
            ? VISUAL_MEMORY_PAGE.Rows[1]
            : {},
      ),
    }),
  );
  const original = runtime.plugins.call;
  let count = 0;
  const call = vi.spyOn(runtime.plugins, "call").mockImplementation((request, options) => {
    if (request.method === "flame.memory/list.v2")
      return list ? (list(request) as never) : original(request, options);
    const key = `memory-command-${++count}`;
    return mutationResult((key) => settle(request, key), key);
  });
  const context: ComponentContext = {
    component: { ...VISUAL_MEMORY_BINDING, componentId: "memory" },
    binding: VISUAL_MEMORY_BINDING,
    runtime,
    workspace: { ref: { path: "/Users/visual/scope" } },
    telemetry: { read: () => ({ spans: [], metrics: [], logs: [] }), clear() {} },
    locale,
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
  const rendered = testClient.render(<Memory context={context} />);
  await screen.findByText(VISUAL_MEMORY_PAGE.Rows[1]!.Content, { exact: true });
  await waitFor(() => expect(screen.queryByText("Reading recorded memory…")).toBeNull());
  return { settle, call, transport, context, rerender: rendered.rerender };
}

const secondPage: MemoryOutputs["flame.memory/list.v2"] = {
  ...VISUAL_MEMORY_PAGE,
  Rows: [
    {
      ...VISUAL_MEMORY_PAGE.Rows[1]!,
      ID: "mem_00000000000000000000000000000004",
      Content: "Older memory kept on the second page.",
    },
  ],
  NextCursor: "",
};

function listed(request: CallPluginRequest): CallPluginResponse {
  const { Cursor } = JSON.parse(request.input) as { Cursor: string };
  return {
    output: JSON.stringify(
      Cursor === "memory-page-2"
        ? secondPage
        : { ...VISUAL_MEMORY_PAGE, NextCursor: "memory-page-2" },
    ),
  };
}

async function openPagedMemory() {
  const list = vi.fn(async (request: CallPluginRequest) => listed(request));
  const opened = await openMemory(list);
  // The established subscription rereads once; navigation must not race that read.
  await waitFor(() => expect(list).toHaveBeenCalledTimes(2));
  await waitFor(() => expect(screen.queryByText("Reading recorded memory…")).toBeNull());
  return { ...opened, list };
}

function listedInputs(list: Mock<(request: CallPluginRequest) => Promise<CallPluginResponse>>) {
  return list.mock.calls.map(
    ([request]) => JSON.parse(request.input) as { Scope: string; Cursor: string },
  );
}

function pageSummary(page: number) {
  return within(catalog()).getByText(new RegExp(`^Page ${page} · `));
}

async function turnTo(name: "Next page" | "Previous page", page: number) {
  const button = screen.getByRole("button", { name });
  button.focus();
  fireEvent.click(button);
  await waitFor(() => expect(document.activeElement).toBe(pageSummary(page)));
  return button;
}

function startDraft() {
  const add = screen.getByRole<HTMLButtonElement>("button", { name: "Add memory" });
  fireEvent.click(add);
  const editor = screen.getByRole("region", { name: "Add memory" });
  const content = within(editor).getByRole<HTMLTextAreaElement>("textbox", {
    name: "Memory content",
  });
  fireEvent.change(content, { target: { value: "Keep my unsaved memory draft." } });
  return { add, editor, content };
}

async function openActions() {
  const trigger = screen.getByRole("button", {
    name: `More actions for ${VISUAL_MEMORY_PAGE.Rows[1]!.ID}`,
  });
  fireEvent.click(trigger);
  await screen.findByRole("menu");
  return trigger;
}

function catalog() {
  return screen.getByRole("heading", { name: "Saved memory" }).closest("section")!;
}

function pressEscape(target: Element, init: KeyboardEventInit = {}) {
  const outer = vi.fn();
  document.addEventListener("keydown", outer);
  const event = new KeyboardEvent("keydown", {
    key: "Escape",
    bubbles: true,
    cancelable: true,
    ...init,
  });
  if (init.keyCode !== undefined) Object.defineProperty(event, "keyCode", { value: init.keyCode });
  fireEvent(target, event);
  document.removeEventListener("keydown", outer);
  return { consumed: event.defaultPrevented, reachedOuter: outer.mock.calls.length > 0 };
}

function commands(call: Awaited<ReturnType<typeof openMemory>>["call"]) {
  return call.mock.calls.filter(([request]) => request.method !== "flame.memory/list.v2");
}

function clickBlocked(button: HTMLElement) {
  expect(button.getAttribute("aria-disabled")).toBe("true");
  expect(button.hasAttribute("disabled")).toBe(false);
  fireEvent.click(button);
}

const active = VISUAL_MEMORY_PAGE.Rows[1]!;

const pending = VISUAL_MEMORY_PAGE.Rows[0]!;

async function chooseFilter(name: string, value: string) {
  fireEvent.click(screen.getByRole("combobox", { name }));
  const option = await screen.findByRole("option", { name: value });
  fireEvent.pointerDown(option, { pointerType: "mouse" });
  fireEvent.click(option);
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
}

it("clears search and every filter locally while keeping the open memory draft", async () => {
  const { call } = await openMemory();
  const { editor, content } = startDraft();
  const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this page" });
  fireEvent.click(screen.getByRole("button", { name: "Filters" }));
  await chooseFilter("Memory status", "Pending review");
  await chooseFilter("Memory origin", "User authored");
  fireEvent.click(screen.getByLabelText("Pinned only", { selector: "input" }));
  fireEvent.change(search, { target: { value: "absent" } });
  fireEvent.click(screen.getByRole("button", { name: "Filters · 3 active" }));
  const issued = call.mock.calls.length;
  const clear = within(catalog()).getByRole("button", { name: "Clear search and filters" });
  clear.focus();
  fireEvent.click(clear);

  expect(search.value).toBe("");
  expect(document.activeElement).toBe(search);
  expect(screen.getByRole("region", { name: "Add memory" })).toBe(editor);
  expect(content.value).toBe("Keep my unsaved memory draft.");
  expect(screen.getByText(active.Content, { exact: true })).toBeTruthy();
  expect(screen.getByText(pending.Content, { exact: true })).toBeTruthy();
  expect(call.mock.calls).toHaveLength(issued);
  fireEvent.click(screen.getByRole("button", { name: "Filters" }));
  expect(screen.getByRole("combobox", { name: "Memory status" }).textContent).toContain(
    "All statuses",
  );
  expect(screen.getByRole("combobox", { name: "Memory origin" }).textContent).toContain(
    "All origins",
  );
  expect(screen.getByRole("checkbox", { name: "Pinned only" }).getAttribute("aria-checked")).toBe(
    "false",
  );
});

it("returns secondary-filter clearing to search without changing its query", async () => {
  const { call } = await openMemory();
  const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this page" });
  fireEvent.change(search, { target: { value: "cutpoint" } });
  fireEvent.click(screen.getByRole("button", { name: "Filters" }));
  fireEvent.click(screen.getByLabelText("Pinned only", { selector: "input" }));
  const issued = call.mock.calls.length;
  const clear = screen.getByRole("button", { name: "Clear filters" });
  clear.focus();
  fireEvent.click(clear);
  expect(document.activeElement).toBe(search);
  expect(search.value).toBe("cutpoint");
  expect(screen.getByText(active.Content, { exact: true })).toBeTruthy();
  expect(screen.queryByText(pending.Content, { exact: true })).toBeNull();
  expect(call.mock.calls).toHaveLength(issued);
});

it.each(["pending", "failed"] as const)(
  "keeps local empty-result recovery available during a %s catalog read",
  async (state) => {
    const { call } = await openMemory();
    const { editor, content } = startDraft();
    const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this page" });
    fireEvent.change(search, { target: { value: "absent" } });
    const read = Promise.withResolvers<CallPluginResponse>();
    call.mockImplementationOnce(() => read.promise as never);
    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
    await screen.findByText("Reading recorded memory…");
    if (state === "failed") {
      await act(async () => read.reject(new Error("Memory catalog is unavailable.")));
      await screen.findByText("Memory catalog is unavailable.");
    }
    const issued = call.mock.calls.length;
    fireEvent.click(within(catalog()).getByRole("button", { name: "Clear search and filters" }));
    expect(search.value).toBe("");
    expect(document.activeElement).toBe(search);
    expect(screen.getByText(active.Content, { exact: true })).toBeTruthy();
    expect(screen.getByText(pending.Content, { exact: true })).toBeTruthy();
    expect(screen.getByRole("region", { name: "Add memory" })).toBe(editor);
    expect(content.value).toBe("Keep my unsaved memory draft.");
    expect(call.mock.calls).toHaveLength(issued);
    if (state === "pending") {
      await act(async () => read.resolve({ output: JSON.stringify(VISUAL_MEMORY_PAGE) }));
    }
    await waitFor(() => expect(screen.queryByText("Reading recorded memory…")).toBeNull());
    expect(document.activeElement).toBe(search);
    expect(screen.getByRole("region", { name: "Add memory" })).toBe(editor);
  },
);

it("keeps the original unknown command and later search focus through local recovery", async () => {
  const { call, settle } = await openMemory();
  const reply = settle.getMockImplementation()!;
  const recovered = Promise.withResolvers<CallPluginResponse>();
  settle
    .mockRejectedValueOnce(new MutationLookupError(new Error("receipt unavailable")))
    .mockImplementationOnce(() => recovered.promise);
  const { editor, content } = startDraft();
  const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this page" });
  fireEvent.change(search, { target: { value: "absent" } });
  fireEvent.click(within(editor).getByRole("button", { name: "Save" }));
  await within(editor).findByText(/result unknown/);
  const issued = call.mock.calls.length;
  fireEvent.click(within(catalog()).getByRole("button", { name: "Clear search and filters" }));
  expect(search.value).toBe("");
  expect(document.activeElement).toBe(search);
  expect(screen.getByRole("region", { name: "Add memory" })).toBe(editor);
  expect(content.value).toBe("Keep my unsaved memory draft.");
  clickBlocked(within(editor).getByRole("button", { name: "Cancel" }));
  expect(screen.getByRole("region", { name: "Add memory" })).toBe(editor);
  expect(call.mock.calls).toHaveLength(issued);
  const reconcile = within(editor).getByRole("button", { name: "Check original result" });
  reconcile.focus();
  fireEvent.click(reconcile);
  await waitFor(() => expect(settle).toHaveBeenCalledTimes(2));
  search.focus();
  await act(async () => recovered.resolve(await reply(...settle.mock.calls[1]!)));
  await within(catalog()).findByText("Add project memory · completed.");
  expect(screen.queryByRole("region", { name: "Add memory" })).toBeNull();
  expect(document.activeElement).toBe(search);
  expect(settle.mock.calls.map(([, key]) => key)).toEqual(["memory-command-1", "memory-command-1"]);
});

it.each(["empty", "unavailable"] as const)(
  "offers no filter reset without records in the %s target",
  async (state) => {
    const { call } = await openMemory();
    const admitted = call.getMockImplementation()!;
    call.mockImplementation((request, options) =>
      request.method === "flame.memory/list.v2"
        ? ((state === "empty"
            ? Promise.resolve({ output: JSON.stringify({ ...VISUAL_MEMORY_PAGE, Rows: [] }) })
            : Promise.reject(new Error("Memory catalog is unavailable."))) as never)
        : admitted(request, options),
    );
    fireEvent.click(screen.getByRole("radio", { name: "User" }));
    await screen.findByText(
      state === "empty" ? "No memory items in this target." : "Memory catalog is unavailable.",
    );
    expect(screen.queryByRole("button", { name: "Clear search and filters" })).toBeNull();
    expect(screen.queryByText(active.Content, { exact: true })).toBeNull();
    expect(screen.queryByText(pending.Content, { exact: true })).toBeNull();
  },
);

it("cancels the local editor on Escape and returns to its opener with the query and filters", async () => {
  await openMemory();
  const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this page" });
  fireEvent.change(search, { target: { value: "compaction" } });
  fireEvent.click(screen.getByRole("button", { name: "Filters" }));
  fireEvent.click(screen.getByRole("combobox", { name: "Memory status" }));
  const activeOption = await screen.findByRole("option", { name: "Active" });
  fireEvent.pointerDown(activeOption, { pointerType: "mouse" });
  fireEvent.click(activeOption);
  await screen.findByRole("button", { name: "Filters · 1 active" });
  const edit = screen.getByRole("button", { name: "Edit" });
  edit.focus();
  fireEvent.click(edit);
  const editor = screen.getByRole("region", { name: "Edit memory" });
  const content = within(editor).getByRole("textbox", { name: "Memory content" });
  await waitFor(() => expect(document.activeElement).toBe(content));
  expect(pressEscape(content)).toEqual({ consumed: true, reachedOuter: false });
  await waitFor(() => expect(screen.queryByRole("region", { name: "Edit memory" })).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(edit));
  expect(search.value).toBe("compaction");
  expect(screen.getByRole("button", { name: "Filters · 1 active" })).toBeTruthy();
  expect(pressEscape(edit)).toEqual({ consumed: false, reachedOuter: true });
});

it("lets a nonempty search and nested choices own Escape before the editor", async () => {
  await openMemory();
  const { add, editor, content } = startDraft();
  fireEvent.click(screen.getByRole("button", { name: "Filters" }));
  const status = screen.getByRole("combobox", { name: "Memory status" });
  fireEvent.click(status);
  const option = await screen.findByRole("option", { name: "All statuses" });
  fireEvent.keyDown(option, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  expect(screen.getByRole("region", { name: "Add memory" })).toBe(editor);
  const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this page" });
  fireEvent.change(search, { target: { value: "compaction" } });
  search.focus();
  expect(pressEscape(search).reachedOuter).toBe(false);
  expect(search.value).toBe("");
  expect(screen.getByRole("region", { name: "Add memory" })).toBe(editor);
  expect(content.value).toBe("Keep my unsaved memory draft.");
  expect(pressEscape(search)).toEqual({ consumed: true, reachedOuter: false });
  await waitFor(() => expect(screen.queryByRole("region", { name: "Add memory" })).toBeNull());
  expect(document.activeElement).toBe(search);
  expect(add.disabled).toBe(false);
});

it.each([
  ["composition", { isComposing: true }],
  ["legacy composition", { keyCode: 229 }],
] as const)("keeps the editor open for %s Escape", async (_, init) => {
  await openMemory();
  const { editor, content } = startDraft();
  expect(pressEscape(content, init).consumed).toBe(false);
  expect(screen.getByRole("region", { name: "Add memory" })).toBe(editor);
});

it("leaves an Escape already handled by a descendant to that owner", async () => {
  await openMemory();
  const { editor, content } = startDraft();
  const event = new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true });
  event.preventDefault();
  fireEvent(content, event);
  expect(screen.getByRole("region", { name: "Add memory" })).toBe(editor);
});

it("consumes Escape without cancelling an unresolved editor command", async () => {
  const { settle } = await openMemory();
  let fail!: (error: unknown) => void;
  settle.mockImplementationOnce(() => new Promise((_, reject) => (fail = reject)));
  const { editor, content } = startDraft();
  fireEvent.click(within(editor).getByRole("button", { name: "Save" }));
  await within(editor).findByText("Add project memory · submitting…");
  expect(pressEscape(content)).toEqual({ consumed: true, reachedOuter: false });
  expect(screen.getByRole("region", { name: "Add memory" })).toBe(editor);
  await act(async () => fail(new MutationLookupError(new Error("receipt unavailable"))));
  await within(editor).findByText(/Add project memory · result unknown/);
  expect(pressEscape(content)).toEqual({ consumed: true, reachedOuter: false });
  expect(screen.getByRole("region", { name: "Add memory" })).toBe(editor);
  expect(content.value).toBe("Keep my unsaved memory draft.");
  fireEvent.click(within(editor).getByRole("button", { name: "Check original result" }));
  await within(catalog()).findByText("Add project memory · completed.");
  expect(screen.queryByRole("region", { name: "Add memory" })).toBeNull();
});

it("keeps one focused primary action through submission, unknown result and recheck", async () => {
  const { settle, call } = await openMemory();
  const reply = settle.getMockImplementation()!;
  const submitted = Promise.withResolvers<CallPluginResponse>();
  const observed = Promise.withResolvers<CallPluginResponse>();
  const recovered = Promise.withResolvers<CallPluginResponse>();
  settle
    .mockImplementationOnce(() => submitted.promise)
    .mockImplementationOnce(() => observed.promise)
    .mockImplementationOnce(() => recovered.promise);
  fireEvent.click(screen.getByRole("button", { name: "Add memory" }));
  const editor = screen.getByRole("region", { name: "Add memory" });
  const save = within(editor).getByRole("button", { name: "Save" });
  expect(save.hasAttribute("disabled")).toBe(true);
  const content = within(editor).getByRole<HTMLTextAreaElement>("textbox", {
    name: "Memory content",
  });
  fireEvent.change(content, { target: { value: "Keep my unsaved memory draft." } });
  save.focus();
  fireEvent.click(save);
  await within(editor).findByText("Add project memory · submitting…");
  expect(within(editor).getByRole("button", { name: "Save" })).toBe(save);
  expect(document.activeElement).toBe(save);
  clickBlocked(save);
  clickBlocked(within(editor).getByRole("button", { name: "Cancel" }));
  expect(pressEscape(save)).toEqual({ consumed: true, reachedOuter: false });
  expect(commands(call)).toHaveLength(1);

  await act(async () =>
    submitted.reject(new MutationLookupError(new Error("receipt unavailable"))),
  );
  await within(editor).findByText("Add project memory · result unknown.");
  expect(within(editor).getByRole("button", { name: "Check original result" })).toBe(save);
  expect(screen.getAllByRole("button", { name: "Check original result" })).toHaveLength(1);
  expect(save.getAttribute("aria-disabled")).not.toBe("true");
  expect(document.activeElement).toBe(save);
  expect(within(editor).getByText("Original command: memory-command-1")).toBeTruthy();
  expect(screen.queryByRole("group", { name: "Original Memory command" })).toBeNull();
  clickBlocked(within(editor).getByRole("button", { name: "Cancel" }));
  expect(pressEscape(save)).toEqual({ consumed: true, reachedOuter: false });
  fireEvent.click(screen.getByRole("button", { name: "Approve" }));
  expect(commands(call)).toHaveLength(1);

  fireEvent.click(save);
  await waitFor(() => expect(settle).toHaveBeenCalledTimes(2));
  expect(within(editor).getByRole("button", { name: "Check original result" })).toBe(save);
  expect(document.activeElement).toBe(save);
  clickBlocked(save);
  expect(pressEscape(save)).toEqual({ consumed: true, reachedOuter: false });
  expect(screen.getByRole("region", { name: "Add memory" })).toBe(editor);
  expect(content.value).toBe("Keep my unsaved memory draft.");
  await act(async () =>
    observed.reject(new MutationLookupError(new Error("receipt still unavailable"))),
  );
  await within(editor).findByText("Add project memory · result unknown.");
  expect(within(editor).getByRole("button", { name: "Check original result" })).toBe(save);
  expect(document.activeElement).toBe(save);
  expect(within(editor).getByText("Original command: memory-command-1")).toBeTruthy();
  fireEvent.click(save);
  await waitFor(() => expect(settle).toHaveBeenCalledTimes(3));
  clickBlocked(save);
  expect(commands(call)).toHaveLength(1);
  const search = screen.getByRole("searchbox", { name: "Search this page" });
  search.focus();
  await act(async () => recovered.resolve(await reply(...settle.mock.calls[2]!)));
  await within(catalog()).findByText("Add project memory · completed.");
  expect(screen.queryByRole("region", { name: "Add memory" })).toBeNull();
  await waitFor(() => expect(screen.queryByText("Reading recorded memory…")).toBeNull());
  expect(document.activeElement).toBe(search);
  expect(settle.mock.calls.map(([, key]) => key)).toEqual([
    "memory-command-1",
    "memory-command-1",
    "memory-command-1",
  ]);
  expect(commands(call)).toHaveLength(1);
});

it("returns a known refusal to the same focused Save for resubmission", async () => {
  const { settle, call } = await openMemory();
  const submitted = Promise.withResolvers<CallPluginResponse>();
  settle.mockImplementationOnce(() => submitted.promise);
  const { editor, content } = startDraft();
  const save = within(editor).getByRole("button", { name: "Save" });
  save.focus();
  fireEvent.click(save);
  await within(editor).findByText("Add project memory · submitting…");
  expect(document.activeElement).toBe(save);
  await act(async () => submitted.reject(new Error("Memory capacity is full.")));
  await within(editor).findByText("Add project memory · request refused.");
  expect(within(editor).getByText("Memory capacity is full.")).toBeTruthy();
  expect(
    within(within(editor).getByRole("group", { name: "Save memory content" })).getByText(
      "Memory capacity is full.",
    ),
  ).toBeTruthy();
  expect(within(editor).queryByText("Review the recorded result, then try again.")).toBeNull();
  expect(within(editor).getByRole("button", { name: "Save" })).toBe(save);
  expect(save.getAttribute("aria-disabled")).not.toBe("true");
  expect(document.activeElement).toBe(save);
  expect(screen.queryByRole("button", { name: "Check original result" })).toBeNull();
  expect(content.value).toBe("Keep my unsaved memory draft.");
  fireEvent.click(save);
  await within(catalog()).findByText("Add project memory · completed.");
  expect(screen.queryByRole("region", { name: "Add memory" })).toBeNull();
  expect(commands(call)).toHaveLength(2);
  expect(settle.mock.calls.map(([, key]) => key)).toEqual(["memory-command-1", "memory-command-2"]);
  expect(JSON.parse(settle.mock.calls[1]![0].input)).toMatchObject({
    Content: "Keep my unsaved memory draft.",
  });
});

it("names the captured target and keeps editor and record feedback apart", async () => {
  const { settle } = await openMemory();
  settle.mockRejectedValueOnce(new Error("Memory capacity is full."));
  fireEvent.click(screen.getByRole("button", { name: "Edit" }));
  const editor = screen.getByRole("region", { name: "Edit memory" });
  expect(within(editor).getByText(active.ID, { exact: true })).toBeTruthy();
  const content = within(editor).getByRole("textbox", { name: "Memory content" });
  fireEvent.change(content, { target: { value: "Keep my edited content." } });
  fireEvent.click(within(editor).getByRole("button", { name: "Save" }));
  await within(editor).findByText(`Edit ${active.ID} · request refused.`);
  expect(within(editor).getByText("Memory capacity is full.")).toBeTruthy();
  expect(within(catalog()).queryByText(/refused/)).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Approve" }));
  await within(catalog()).findByText(`Approve ${pending.ID} · completed.`);
  expect(within(editor).queryByText(/Approve/)).toBeNull();
  expect(within(editor).getByText(`Edit ${active.ID} · request refused.`)).toBeTruthy();
  expect(screen.getByRole("region", { name: "Edit memory" })).toBe(editor);
  expect(content).toHaveProperty("value", "Keep my edited content.");
  fireEvent.click(within(editor).getByRole("button", { name: "Save" }));
  await within(catalog()).findByText(`Edit ${active.ID} · completed.`);
  expect(screen.queryByRole("region", { name: "Edit memory" })).toBeNull();
  expect(settle.mock.calls.map(([request]) => JSON.parse(request.input))).toEqual([
    { ID: active.ID, ExpectedRevision: active.Revision, Content: "Keep my edited content." },
    { ID: pending.ID, ExpectedRevision: pending.Revision, Decision: "approve" },
    { ID: active.ID, ExpectedRevision: active.Revision, Content: "Keep my edited content." },
  ]);
});

const userMemory: typeof active = {
  ...active,
  ID: "mem_00000000000000000000000000000005",
  Revision: 12,
  Scope: "user",
  Origin: "user",
  Pinned: false,
  SessionID: "",
  Content: "Answer my questions in concise English.",
};

function facts(row: Element) {
  return Object.fromEntries(
    [...row.querySelectorAll("dl > div")].map((fact) => [
      fact.querySelector("dt")!.textContent,
      fact.querySelector("dd")!.textContent,
    ]),
  );
}

const englishScope = {
  locale: "en",
  saved: "Saved memory",
  reading: "Reading recorded memory…",
  segment: "User",
  details: "Record details",
  scopeFact: "Scope",
  edit: "Edit",
  editor: "Edit memory",
  content: "Memory content",
  cancel: "Cancel",
  save: "Save",
  more: (id: string) => `More actions for ${id}`,
  delete: "Delete",
  confirmation: "Confirm memory deletion",
  edited: (id: string) => `Edit ${id} · completed.`,
};

const chineseScope: typeof englishScope = {
  locale: "zh",
  saved: "已保存的记忆",
  reading: "正在读取已记录的记忆…",
  segment: "用户",
  details: "记录详情",
  scopeFact: "范围",
  edit: "编辑",
  editor: "编辑记忆",
  content: "记忆内容",
  cancel: "取消",
  save: "保存",
  more: (id: string) => `${id} 的更多操作`,
  delete: "删除",
  confirmation: "确认删除记忆",
  edited: (id: string) => `编辑 ${id} · 已完成。`,
};

it.each([
  {
    ...englishScope,
    target: "Project memory · /Users/visual/scope",
    scope: "Project memory",
    item: active,
    cwd: "/Users/visual/scope",
  },
  { ...englishScope, target: "User memory", scope: "User memory", item: userMemory, cwd: "" },
  {
    ...chineseScope,
    target: "项目记忆 · /Users/visual/scope",
    scope: "项目记忆",
    item: active,
    cwd: "/Users/visual/scope",
  },
  { ...chineseScope, target: "用户记忆", scope: "用户记忆", item: userMemory, cwd: "" },
])("names each $target record by its captured scope in $locale", async (copy) => {
  const { item } = copy;
  const rows = { project: active, user: userMemory };
  const list = vi.fn(async (request: CallPluginRequest) => {
    const { Scope } = JSON.parse(request.input) as { Scope: keyof typeof rows };
    return { output: JSON.stringify({ ...VISUAL_MEMORY_PAGE, Rows: [rows[Scope]] }) };
  });
  const { settle, call } = await openMemory(list, copy.locale);
  await waitFor(() => expect(list).toHaveBeenCalledTimes(2));
  const targetReads = item.Scope === "user" ? 2 : 0;
  if (item.Scope === "user") {
    fireEvent.click(screen.getByRole("radio", { name: copy.segment }));
  }
  await waitFor(() => expect(list).toHaveBeenCalledTimes(2 + targetReads));
  await waitFor(() => expect(screen.queryByText(copy.reading)).toBeNull());
  const records = () => screen.getByRole("heading", { name: copy.saved }).closest("section")!;
  await within(records()).findByText(item.Content, { exact: true });
  expect(screen.getByText(copy.target, { exact: true })).toBeTruthy();
  const row = () => records().querySelector<HTMLElement>(`[data-memory-id="${item.ID}"]`)!;
  fireEvent.click(within(row()).getByRole("button", { name: new RegExp(`^${copy.details}`) }));
  await waitFor(() =>
    expect(facts(row())).toMatchObject({ ID: item.ID, [copy.scopeFact]: copy.scope }),
  );
  expect(Object.values(facts(row()))).not.toContain(item.Scope);

  const edit = within(row()).getByRole("button", { name: copy.edit });
  edit.focus();
  fireEvent.click(edit);
  const editor = screen.getByRole("region", { name: copy.editor });
  const editorScope = within(editor).getByText(copy.scope, { exact: true });
  expect(within(editor).getByText(item.ID, { exact: true }).parentElement).toBe(
    editorScope.parentElement,
  );
  expect(within(editor).queryByText(item.Scope, { exact: true })).toBeNull();
  fireEvent.change(within(editor).getByRole("textbox", { name: copy.content }), {
    target: { value: "Discard this memory edit." },
  });
  const cancelEdit = within(editor).getByRole("button", { name: copy.cancel });
  cancelEdit.focus();
  fireEvent.click(cancelEdit);
  await waitFor(() => expect(document.activeElement).toBe(edit));
  expect(screen.queryByRole("region", { name: copy.editor })).toBeNull();
  expect(within(records()).getByText(item.Content, { exact: true })).toBeTruthy();

  const trigger = within(row()).getByRole("button", { name: copy.more(item.ID) });
  fireEvent.click(trigger);
  await screen.findByRole("menu");
  fireEvent.click(screen.getByRole("menuitem", { name: copy.delete }));
  const confirmation = screen.getByRole("region", { name: copy.confirmation });
  const deletionScope = within(confirmation).getByText(copy.scope, { exact: true });
  expect(within(confirmation).getByText(item.ID, { exact: true }).parentElement).toBe(
    deletionScope.parentElement,
  );
  const cancelDelete = within(confirmation).getByRole("button", { name: copy.cancel });
  cancelDelete.focus();
  fireEvent.click(cancelDelete);
  await waitFor(() => expect(document.activeElement).toBe(trigger));
  await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
  expect(screen.queryByRole("region", { name: copy.confirmation })).toBeNull();
  expect(commands(call)).toHaveLength(0);

  const content = "Keep this edited memory in its original scope.";
  settle.mockImplementationOnce(async () => {
    rows[item.Scope] = { ...item, Revision: Number(item.Revision) + 1, Content: content };
    return { output: JSON.stringify(rows[item.Scope]) };
  });
  fireEvent.click(within(row()).getByRole("button", { name: copy.edit }));
  const reopened = screen.getByRole("region", { name: copy.editor });
  const field = within(reopened).getByRole<HTMLTextAreaElement>("textbox", {
    name: copy.content,
  });
  expect(field.value).toBe(item.Content);
  fireEvent.change(field, { target: { value: content } });
  const settledReads = list.mock.calls.length;
  fireEvent.click(within(reopened).getByRole("button", { name: copy.save }));
  await within(records()).findByText(copy.edited(item.ID));
  await waitFor(() => expect(list.mock.calls.length).toBeGreaterThan(settledReads));
  await within(records()).findByText(content, { exact: true });
  await waitFor(() => expect(screen.queryByText(copy.reading)).toBeNull());
  expect(within(records()).queryByText(item.Content, { exact: true })).toBeNull();
  expect(facts(row())).toMatchObject({ ID: item.ID, [copy.scopeFact]: copy.scope });
  expect(commands(call)).toHaveLength(1);
  expect(commands(call)[0]![0].method).toBe("flame.memory/update.v2");
  expect(JSON.parse(commands(call)[0]![0].input)).toEqual({
    ID: item.ID,
    ExpectedRevision: item.Revision,
    Content: content,
  });
  const reads = list.mock.calls.slice(targetReads).map(([request]) => JSON.parse(request.input));
  expect(reads).toEqual(Array(reads.length).fill({ Scope: item.Scope, CWD: copy.cwd, Cursor: "" }));
});

it("does not carry a cancelled task's refusal into a new task", async () => {
  const { settle } = await openMemory();
  settle.mockRejectedValueOnce(new Error("Memory capacity is full."));
  fireEvent.click(screen.getByRole("button", { name: "Edit" }));
  const editor = screen.getByRole("region", { name: "Edit memory" });
  fireEvent.change(within(editor).getByRole("textbox", { name: "Memory content" }), {
    target: { value: "Keep my edited content." },
  });
  fireEvent.click(within(editor).getByRole("button", { name: "Save" }));
  await screen.findByText("Memory capacity is full.");
  fireEvent.click(within(editor).getByRole("button", { name: "Cancel" }));
  startDraft();
  expect(screen.queryByText("Memory capacity is full.")).toBeNull();
  expect(screen.queryByText(/refused/)).toBeNull();
});

it("keeps a draft and its actions after an independent record refusal", async () => {
  const { settle } = await openMemory();
  settle.mockRejectedValueOnce(new Error("Memory was already reviewed."));
  const { editor, content } = startDraft();
  fireEvent.click(screen.getByRole("button", { name: "Approve" }));
  await within(catalog()).findByText(`Approve ${pending.ID} · request refused.`);
  expect(within(catalog()).getByText("Memory was already reviewed.")).toBeTruthy();
  expect(within(editor).queryByText(/refused/)).toBeNull();
  expect(screen.getByRole("region", { name: "Add memory" })).toBe(editor);
  expect(content.value).toBe("Keep my unsaved memory draft.");
  expect(within(editor).getByRole<HTMLButtonElement>("button", { name: "Save" }).disabled).toBe(
    false,
  );
});

it("returns to retained search when the captured opener is disabled by a read", async () => {
  const { call } = await openMemory();
  const edit = screen.getByRole<HTMLButtonElement>("button", { name: "Edit" });
  fireEvent.click(edit);
  const editor = screen.getByRole("region", { name: "Edit memory" });
  const cancel = within(editor).getByRole("button", { name: "Cancel" });
  cancel.focus();
  call.mockImplementationOnce(() => new Promise(() => {}) as never);
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await screen.findByText("Reading recorded memory…");
  fireEvent.click(cancel);
  expect(edit.disabled).toBe(true);
  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByRole("searchbox", { name: "Search this page" }),
    ),
  );
});

it("settles authoring before the catalog read and preserves a later focus choice", async () => {
  const { call } = await openMemory();
  const admitted = call.getMockImplementation()!;
  const refresh = Promise.withResolvers<CallPluginResponse>();
  call.mockImplementation((request, options) =>
    request.method === "flame.memory/list.v2"
      ? (refresh.promise as never)
      : admitted(request, options),
  );
  const { add, editor } = startDraft();
  const save = within(editor).getByRole("button", { name: "Save" });
  save.focus();
  fireEvent.click(save);
  await screen.findByText("Reading recorded memory…");
  await within(catalog()).findByText("Add project memory · completed.");
  expect(screen.queryByRole("region", { name: "Add memory" })).toBeNull();
  expect(add.disabled).toBe(false);
  await waitFor(() => expect(document.activeElement).toBe(add));
  const search = screen.getByRole("searchbox", { name: "Search this page" });
  search.focus();
  await act(async () => refresh.resolve({ output: JSON.stringify(VISUAL_MEMORY_PAGE) }));
  await waitFor(() => expect(screen.queryByText("Reading recorded memory…")).toBeNull());
  expect(document.activeElement).toBe(search);
});

it("keeps known results visible without a catalog and retires notices with their scope", async () => {
  const { call, settle } = await openMemory();
  fireEvent.click(screen.getByRole("button", { name: "Approve" }));
  await within(catalog()).findByText(`Approve ${pending.ID} · completed.`);
  const admitted = call.getMockImplementation()!;
  call.mockImplementation((request, options) =>
    request.method === "flame.memory/list.v2"
      ? (Promise.reject(new Error("Memory catalog is unavailable.")) as never)
      : admitted(request, options),
  );
  fireEvent.click(screen.getByRole("radio", { name: "User" }));
  await screen.findByText("Memory catalog is unavailable.");
  expect(screen.queryByText(`Approve ${pending.ID} · completed.`)).toBeNull();
  settle.mockResolvedValueOnce({
    output: JSON.stringify({
      Item: { ...active, Scope: "user", Content: "Keep my unsaved memory draft." },
      Changed: true,
    } satisfies MemoryOutputs["flame.memory/add.v2"]),
  });
  const { editor } = startDraft();
  fireEvent.click(within(editor).getByRole("button", { name: "Save" }));
  await screen.findByRole("heading", { name: "Saved memory" });
  await within(catalog()).findByText("Add user memory · completed.");
  expect(screen.queryByText("No memory items in this target.")).toBeNull();
  expect(JSON.parse(settle.mock.calls.at(-1)![0].input)).toMatchObject({
    Scope: "user",
    CWD: "",
  });
});

it("keeps one memory form and its target until explicit cancellation", async () => {
  await openMemory();
  const { add, editor, content } = startDraft();
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "Edit" }).disabled).toBe(true);
  expect(screen.getByRole("radio", { name: "User" }).hasAttribute("disabled")).toBe(true);
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "Approve" }).disabled).toBe(false);
  const actions = await openActions();
  expect(screen.getByRole("menuitem", { name: "Delete" }).getAttribute("aria-disabled")).toBe(
    "true",
  );
  expect(screen.getByRole("menuitem", { name: "Unpin" }).getAttribute("aria-disabled")).not.toBe(
    "true",
  );
  fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(actions));
  expect(content.value).toBe("Keep my unsaved memory draft.");
  const cancel = within(editor).getByRole("button", { name: "Cancel" });
  cancel.focus();
  fireEvent.click(cancel);
  await waitFor(() => expect(document.activeElement).toBe(add));
  expect(screen.queryByRole("region", { name: "Add memory" })).toBeNull();
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "Edit" }).disabled).toBe(false);
});

it.each(["approve", "unpin"] as const)(
  "preserves a draft after an independent %s succeeds",
  async (action) => {
    const { settle } = await openMemory();
    const { editor, content } = startDraft();
    if (action === "approve") fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    else {
      await openActions();
      fireEvent.click(screen.getByRole("menuitem", { name: "Unpin" }));
    }
    await within(catalog()).findByText(
      action === "approve"
        ? `Approve ${pending.ID} · completed.`
        : `Unpin ${active.ID} · completed.`,
    );
    expect(settle).toHaveBeenCalledOnce();
    expect(screen.getByRole("region", { name: "Add memory" })).toBe(editor);
    expect(within(editor).getByRole("textbox", { name: "Memory content" })).toBe(content);
    expect(content.value).toBe("Keep my unsaved memory draft.");
  },
);

it("closes a submitted draft only after its own command succeeds", async () => {
  const { settle } = await openMemory();
  const { add, editor } = startDraft();
  fireEvent.click(within(editor).getByRole("button", { name: "Save" }));
  await within(catalog()).findByText("Add project memory · completed.");
  expect(settle.mock.calls[0]![0].method).toBe("flame.memory/add.v2");
  expect(JSON.parse(settle.mock.calls[0]![0].input)).toMatchObject({
    Content: "Keep my unsaved memory draft.",
  });
  expect(screen.queryByRole("region", { name: "Add memory" })).toBeNull();
  await waitFor(() => expect(document.activeElement).toBe(add));
});

it("retains a refused draft for correction", async () => {
  const { settle } = await openMemory();
  settle.mockRejectedValueOnce(new Error("Memory capacity is full."));
  const { editor, content } = startDraft();
  fireEvent.click(within(editor).getByRole("button", { name: "Save" }));
  await screen.findByText("Memory capacity is full.");
  expect(content.value).toBe("Keep my unsaved memory draft.");
  expect(within(editor).getByRole<HTMLButtonElement>("button", { name: "Cancel" }).disabled).toBe(
    false,
  );
});

it("distinguishes a record pin from saving content for the same memory", async () => {
  const { settle } = await openMemory();
  fireEvent.click(screen.getByRole("button", { name: "Edit" }));
  const editor = screen.getByRole("region", { name: "Edit memory" });
  const content = within(editor).getByRole<HTMLTextAreaElement>("textbox", {
    name: "Memory content",
  });
  expect(content.value).toBe(VISUAL_MEMORY_PAGE.Rows[1]!.Content);
  fireEvent.change(content, { target: { value: "Keep my edited content." } });
  await openActions();
  fireEvent.click(screen.getByRole("menuitem", { name: "Unpin" }));
  await within(catalog()).findByText(`Unpin ${active.ID} · completed.`);
  expect(screen.getByRole("region", { name: "Edit memory" })).toBe(editor);
  expect(content.value).toBe("Keep my edited content.");
  await waitFor(() =>
    expect(within(editor).getByRole<HTMLButtonElement>("button", { name: "Save" }).disabled).toBe(
      false,
    ),
  );
  fireEvent.click(within(editor).getByRole("button", { name: "Save" }));
  await waitFor(() => expect(screen.queryByRole("region", { name: "Edit memory" })).toBeNull());
  expect(settle.mock.calls.map(([request]) => request.method)).toEqual([
    "flame.memory/update.v2",
    "flame.memory/update.v2",
  ]);
  expect(settle.mock.calls.map(([request]) => JSON.parse(request.input))).toEqual([
    {
      ID: VISUAL_MEMORY_PAGE.Rows[1]!.ID,
      ExpectedRevision: VISUAL_MEMORY_PAGE.Rows[1]!.Revision,
      Pinned: false,
    },
    {
      ID: VISUAL_MEMORY_PAGE.Rows[1]!.ID,
      ExpectedRevision: VISUAL_MEMORY_PAGE.Rows[1]!.Revision,
      Content: "Keep my edited content.",
    },
  ]);
});

it.each(["editor", "record"] as const)(
  "retains the %s completion intent through original-command recovery",
  async (source) => {
    const { settle, call } = await openMemory();
    settle.mockRejectedValueOnce(new MutationLookupError(new Error("receipt unavailable")));
    const { editor, content } = startDraft();
    fireEvent.click(
      source === "editor"
        ? within(editor).getByRole("button", { name: "Save" })
        : screen.getByRole("button", { name: "Approve" }),
    );
    await screen.findByText(/result unknown/);
    expect(content.value).toBe("Keep my unsaved memory draft.");
    clickBlocked(within(editor).getByRole("button", { name: "Cancel" }));
    expect(screen.getByRole("region", { name: "Add memory" })).toBe(editor);
    const [recover] = screen.getAllByRole("button", { name: "Check original result" });
    expect(screen.getAllByRole("button", { name: "Check original result" })).toHaveLength(1);
    expect(editor.contains(recover!)).toBe(source === "editor");
    expect(
      within(catalog()).queryByRole("group", { name: "Original Memory command" }) !== null,
    ).toBe(source === "record");
    expect(screen.getByText("Original command: memory-command-1")).toBeTruthy();
    if (source === "record") {
      clickBlocked(within(editor).getByRole("button", { name: "Save" }));
    }
    expect(pressEscape(content)).toEqual({ consumed: true, reachedOuter: false });
    expect(screen.getByRole("region", { name: "Add memory" })).toBe(editor);
    expect(commands(call)).toHaveLength(1);
    fireEvent.click(recover!);
    await within(catalog()).findByText(
      source === "editor"
        ? "Add project memory · completed."
        : `Approve ${pending.ID} · completed.`,
    );
    expect(settle.mock.calls.map(([, key]) => key)).toEqual([
      "memory-command-1",
      "memory-command-1",
    ]);
    expect(
      call.mock.calls.filter(([request]) => request.method !== "flame.memory/list.v2"),
    ).toHaveLength(1);
    expect(screen.queryByRole("region", { name: "Add memory" }) !== null).toBe(source === "record");
    expect(content.value).toBe("Keep my unsaved memory draft.");
  },
);

async function approveUntilUnknown(...rechecks: Array<Promise<CallPluginResponse>>) {
  const opened = await openMemory();
  const reply = opened.settle.getMockImplementation()!;
  opened.settle.mockRejectedValueOnce(new MutationLookupError(new Error("receipt unavailable")));
  for (const recheck of rechecks) opened.settle.mockImplementationOnce(() => recheck);
  const draft = startDraft();
  fireEvent.click(screen.getByRole("button", { name: "Approve" }));
  const unknown = await within(catalog()).findByText(`Approve ${pending.ID} · result unknown.`);
  const result = unknown.closest("section")!;
  const recover = within(catalog()).getByRole("button", { name: "Check original result" });
  return { ...opened, ...draft, reply, result, recover };
}

it.each(["confirmed", "refused"] as const)(
  "hands a focused record recovery to its %s result while the draft stays",
  async (outcome) => {
    const observed = Promise.withResolvers<CallPluginResponse>();
    const recovered = Promise.withResolvers<CallPluginResponse>();
    const { settle, call, reply, editor, content, result, recover } = await approveUntilUnknown(
      observed.promise,
      recovered.promise,
    );
    expect(document.activeElement).not.toBe(result);
    expect(
      within(catalog()).getByRole("group", { name: "Original Memory command" }).contains(recover),
    ).toBe(true);
    expect(within(editor).queryByRole("button", { name: "Check original result" })).toBeNull();
    recover.focus();
    fireEvent.click(recover);
    await waitFor(() => expect(settle).toHaveBeenCalledTimes(2));
    expect(within(catalog()).getByRole("button", { name: "Check original result" })).toBe(recover);
    expect(document.activeElement).toBe(recover);
    clickBlocked(recover);

    await act(async () =>
      observed.reject(new MutationLookupError(new Error("receipt still unavailable"))),
    );
    await waitFor(() => expect(recover.getAttribute("aria-disabled")).not.toBe("true"));
    expect(within(catalog()).getByRole("button", { name: "Check original result" })).toBe(recover);
    expect(document.activeElement).toBe(recover);
    expect(
      within(catalog()).getByText(`Approve ${pending.ID} · result unknown.`).closest("section"),
    ).toBe(result);
    fireEvent.click(recover);
    await waitFor(() => expect(settle).toHaveBeenCalledTimes(3));
    expect(document.activeElement).toBe(recover);

    await act(async () =>
      outcome === "confirmed"
        ? recovered.resolve(await reply(...settle.mock.calls[2]!))
        : recovered.reject(new Error("Memory was already reviewed.")),
    );
    await within(catalog()).findByText(
      `Approve ${pending.ID} · ${outcome === "confirmed" ? "completed" : "request refused"}.`,
    );
    expect(screen.queryByRole("button", { name: "Check original result" })).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(result));
    expect(result.getAttribute("role")).toBe(outcome === "confirmed" ? "status" : "alert");
    expect(within(result).queryByText("Memory was already reviewed.") !== null).toBe(
      outcome === "refused",
    );
    await waitFor(() => expect(screen.queryByText("Reading recorded memory…")).toBeNull());
    expect(document.activeElement).toBe(result);
    expect(screen.getByRole("region", { name: "Add memory" })).toBe(editor);
    expect(content.value).toBe("Keep my unsaved memory draft.");
    expect(within(editor).getByRole("button", { name: "Save" })).toBeTruthy();
    expect(settle.mock.calls.map(([, key]) => key)).toEqual([
      "memory-command-1",
      "memory-command-1",
      "memory-command-1",
    ]);
    expect(commands(call)).toHaveLength(1);
  },
);

it.each(["search", "draft"] as const)(
  "leaves a later %s focus in place when record recovery settles",
  async (later) => {
    const recovered = Promise.withResolvers<CallPluginResponse>();
    const { settle, call, reply, editor, content, recover } = await approveUntilUnknown(
      recovered.promise,
    );
    recover.focus();
    fireEvent.click(recover);
    await waitFor(() => expect(settle).toHaveBeenCalledTimes(2));
    const chosen =
      later === "search" ? screen.getByRole("searchbox", { name: "Search this page" }) : content;
    chosen.focus();
    await act(async () => recovered.resolve(await reply(...settle.mock.calls[1]!)));
    await within(catalog()).findByText(`Approve ${pending.ID} · completed.`);
    await waitFor(() => expect(screen.queryByText("Reading recorded memory…")).toBeNull());
    expect(document.activeElement).toBe(chosen);
    expect(screen.getByRole("region", { name: "Add memory" })).toBe(editor);
    expect(content.value).toBe("Keep my unsaved memory draft.");
    expect(settle.mock.calls.map(([, key]) => key)).toEqual([
      "memory-command-1",
      "memory-command-1",
    ]);
    expect(commands(call)).toHaveLength(1);
  },
);

async function reviewPendingOnly(name: "Approve" | "Reject" = "Approve") {
  let page: MemoryOutputs["flame.memory/list.v2"] = VISUAL_MEMORY_PAGE;
  const accepted = async (): Promise<CallPluginResponse> => ({ output: JSON.stringify(page) });
  const list = vi.fn(accepted);
  const opened = await openMemory(list);
  await waitFor(() => expect(list).toHaveBeenCalledTimes(2));
  await waitFor(() => expect(screen.queryByText("Reading recorded memory…")).toBeNull());
  fireEvent.click(screen.getByRole("button", { name: "Filters" }));
  await chooseFilter("Memory status", "Pending review");
  const draft = startDraft();
  const reply = opened.settle.getMockImplementation()!;
  const decided = Promise.withResolvers<CallPluginResponse>();
  opened.settle.mockImplementationOnce(() => decided.promise);
  const control = within(catalog()).getByRole("button", { name });
  control.focus();
  fireEvent.click(control);
  await waitFor(() => expect(opened.settle).toHaveBeenCalledOnce());
  page = {
    ...VISUAL_MEMORY_PAGE,
    Rows:
      name === "Reject"
        ? VISUAL_MEMORY_PAGE.Rows.filter((row) => row.ID !== pending.ID)
        : VISUAL_MEMORY_PAGE.Rows.map((row) =>
            row.ID === pending.ID ? { ...row, Status: "active" as const } : row,
          ),
  };
  return { ...opened, ...draft, list, reply, decided, control, accepted };
}

it.each([
  ["Approve", "confirmed"],
  ["Approve", "refused"],
  ["Approve", "unknown"],
  ["Reject", "confirmed"],
  ["Reject", "refused"],
  ["Reject", "unknown"],
] as const)(
  "keeps a focused %s waiting and hands its %s outcome to the catalog result",
  async (name, outcome) => {
    const { settle, call, list, reply, decided, control, editor, content, accepted } =
      await reviewPendingOnly(name);
    const reject = within(catalog()).getByRole("button", {
      name: name === "Approve" ? "Reject" : "Approve",
    });
    expect(document.activeElement).toBe(control);
    clickBlocked(control);
    fireEvent.keyDown(control, { key: "Enter" });
    fireEvent.keyDown(control, { key: " " });
    fireEvent.keyUp(control, { key: " " });
    clickBlocked(reject);
    expect(commands(call)).toHaveLength(1);
    expect(document.activeElement).toBe(control);
    const result = within(catalog())
      .getByText(`${name} ${pending.ID} · submitting…`)
      .closest("section")!;
    const read = Promise.withResolvers<CallPluginResponse>();
    if (outcome === "confirmed") list.mockImplementationOnce(() => read.promise);

    await act(async () =>
      outcome === "confirmed"
        ? decided.resolve(await reply(...settle.mock.calls[0]!))
        : decided.reject(
            outcome === "refused"
              ? new Error("Memory was already reviewed.")
              : new MutationLookupError(new Error("receipt unavailable")),
          ),
    );
    await within(catalog()).findByText(
      `${name} ${pending.ID} · ${
        outcome === "unknown"
          ? "result unknown"
          : outcome === "confirmed"
            ? "completed"
            : "request refused"
      }.`,
    );
    expect(document.activeElement).toBe(result);
    expect(result.getAttribute("role")).toBe(outcome === "refused" ? "alert" : "status");
    expect(within(catalog()).getByRole("button", { name })).toBe(control);
    if (outcome === "confirmed") await act(async () => read.resolve(await accepted()));
    await waitFor(() => expect(screen.queryByText("Reading recorded memory…")).toBeNull());
    expect(control.isConnected).toBe(outcome !== "confirmed");
    expect(within(catalog()).queryByText(pending.Content, { exact: true }) !== null).toBe(
      outcome !== "confirmed",
    );
    expect(within(result).queryByText("Memory was already reviewed.") !== null).toBe(
      outcome === "refused",
    );
    const original = within(catalog()).queryByRole("group", { name: "Original Memory command" });
    expect(original !== null).toBe(outcome === "unknown");
    const key = within(catalog()).queryByText("Original command: memory-command-1");
    expect(key !== null).toBe(outcome === "unknown");
    expect(key !== null && original?.contains(key)).toBe(outcome === "unknown");
    expect(
      within(catalog()).queryByRole("button", { name: "Check original result" }) !== null,
    ).toBe(outcome === "unknown");
    expect(original !== null && result.parentElement!.contains(original)).toBe(
      outcome === "unknown",
    );
    expect(document.activeElement).toBe(result);
    expect(screen.getByRole("region", { name: "Add memory" })).toBe(editor);
    expect(content.value).toBe("Keep my unsaved memory draft.");
    expect(commands(call)).toHaveLength(1);
    expect(JSON.parse(commands(call)[0]![0].input)).toEqual({
      ID: pending.ID,
      ExpectedRevision: pending.Revision,
      Decision: name === "Approve" ? "approve" : "reject",
    });
  },
);

it.each([
  {
    locale: "zh",
    reject: "拒绝",
    saved: "已保存的记忆",
    reading: "正在读取已记录的记忆…",
    submitting: "提交中…",
    refused: "请求被拒绝。",
    unknown: "结果未知。",
    completed: "已完成。",
    ambiguous: "已拒绝。",
    original: "原始记忆命令",
    key: "原始命令：memory-command-2",
    check: "查看原始结果",
  },
  {
    locale: "zh-TW",
    reject: "拒絕",
    saved: "已儲存的記憶",
    reading: "正在讀取已記錄的記憶…",
    submitting: "提交中…",
    refused: "請求被拒絕。",
    unknown: "結果未知。",
    completed: "已完成。",
    ambiguous: "已拒絕。",
    original: "原始記憶命令",
    key: "原始命令：memory-command-2",
    check: "查看原始結果",
  },
])("names a refused $locale Reject request apart from its recovered completion", async (copy) => {
  let page = VISUAL_MEMORY_PAGE;
  const list = vi.fn(async () => ({ output: JSON.stringify(page) }));
  const { settle, call, transport } = await openMemory(list, copy.locale);
  await waitFor(() => expect(list).toHaveBeenCalledTimes(2));
  await waitFor(() => expect(screen.queryByText(copy.reading)).toBeNull());
  const records = () => screen.getByRole("heading", { name: copy.saved }).closest("section")!;
  const label = `${copy.reject} ${pending.ID}`;
  settle.mockRejectedValueOnce(new Error("Memory was already reviewed."));
  fireEvent.click(within(records()).getByRole("button", { name: copy.reject }));
  const refusal = (await within(records()).findByText(`${label} · ${copy.refused}`)).closest(
    "section",
  )!;
  expect(refusal.getAttribute("role")).toBe("alert");
  expect(within(refusal).getByText("Memory was already reviewed.")).toBeTruthy();
  expect(screen.queryByText(`${label} · ${copy.ambiguous}`)).toBeNull();
  expect(screen.getByText(pending.Content, { exact: true })).toBeTruthy();

  const decided = Promise.withResolvers<CallPluginResponse>();
  settle.mockImplementationOnce(() => decided.promise);
  const control = within(records()).getByRole("button", { name: copy.reject });
  fireEvent.click(control);
  await within(records()).findByText(`${label} · ${copy.submitting}`);
  page = { ...page, Rows: page.Rows.filter((row) => row.ID !== pending.ID) };
  transport.removeSchedule("sch_nightly");
  await waitFor(() => expect(control.isConnected).toBe(false));
  expect(within(records()).getByText(`${label} · ${copy.submitting}`)).toBeTruthy();
  expect(screen.queryByText(`${label} · ${copy.completed}`)).toBeNull();
  await act(async () => decided.reject(new MutationLookupError(new Error("receipt unavailable"))));
  const unknown = (await within(records()).findByText(`${label} · ${copy.unknown}`)).closest(
    "section",
  )!;
  expect(unknown.getAttribute("role")).toBe("status");
  expect(
    within(within(records()).getByRole("group", { name: copy.original })).getByText(copy.key),
  ).toBeTruthy();
  fireEvent.click(within(records()).getByRole("button", { name: copy.check }));
  const completed = (await within(records()).findByText(`${label} · ${copy.completed}`)).closest(
    "section",
  )!;
  expect(completed.getAttribute("role")).toBe("status");
  expect(screen.queryByRole("button", { name: copy.check })).toBeNull();
  expect(screen.queryByText("Memory was already reviewed.")).toBeNull();
  expect(settle.mock.calls.map(([, key]) => key)).toEqual([
    "memory-command-1",
    "memory-command-2",
    "memory-command-2",
  ]);
  expect(commands(call)).toHaveLength(2);
  expect(settle.mock.calls.map(([request]) => JSON.parse(request.input))).toEqual(
    Array(3).fill({ ID: pending.ID, ExpectedRevision: pending.Revision, Decision: "reject" }),
  );
});

it.each(["search", "draft"] as const)(
  "leaves a later %s focus in place when a review settles and its row leaves",
  async (later) => {
    const { settle, call, reply, decided, control, editor, content } = await reviewPendingOnly();
    const chosen =
      later === "search" ? screen.getByRole("searchbox", { name: "Search this page" }) : content;
    chosen.focus();
    await act(async () => decided.resolve(await reply(...settle.mock.calls[0]!)));
    await within(catalog()).findByText(`Approve ${pending.ID} · completed.`);
    await waitFor(() => expect(control.isConnected).toBe(false));
    await waitFor(() => expect(screen.queryByText("Reading recorded memory…")).toBeNull());
    expect(document.activeElement).toBe(chosen);
    expect(screen.getByRole("region", { name: "Add memory" })).toBe(editor);
    expect(content.value).toBe("Keep my unsaved memory draft.");
    expect(commands(call)).toHaveLength(1);
  },
);

it.each(["action", "search", "draft"] as const)(
  "keeps the %s reading choice when a subscription removes a pending command's row",
  async (later) => {
    const { transport, call, decided, control, editor, content } = await reviewPendingOnly();
    const chosen =
      later === "search"
        ? screen.getByRole("searchbox", { name: "Search this page" })
        : later === "draft"
          ? content
          : control;
    chosen.focus();
    const result = within(catalog())
      .getByText(`Approve ${pending.ID} · submitting…`)
      .closest("section")!;
    transport.removeSchedule("sch_nightly");
    await waitFor(() => expect(control.isConnected).toBe(false));
    expect(document.activeElement).toBe(later === "action" ? result : chosen);
    await act(async () =>
      decided.reject(new MutationLookupError(new Error("receipt unavailable"))),
    );
    await within(catalog()).findByText(`Approve ${pending.ID} · result unknown.`);
    expect(document.activeElement).toBe(later === "action" ? result : chosen);
    expect(screen.getByRole("region", { name: "Add memory" })).toBe(editor);
    expect(content.value).toBe("Keep my unsaved memory draft.");
    expect(commands(call)).toHaveLength(1);
  },
);

it("does not attach an unrelated prior result to a background record removal", async () => {
  let page = VISUAL_MEMORY_PAGE;
  const list = vi.fn(async () => ({ output: JSON.stringify(page) }));
  const { settle, call, transport } = await openMemory(list);
  await waitFor(() => expect(list).toHaveBeenCalledTimes(2));
  settle.mockRejectedValueOnce(new Error("Pinned item changed."));
  await openActions();
  fireEvent.click(screen.getByRole("menuitem", { name: "Unpin" }));
  const notice = await within(catalog()).findByText(`Unpin ${active.ID} · request refused.`);
  await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
  const unrelated = notice.closest("section");
  const control = screen.getByRole("button", { name: "Approve" });
  control.focus();
  page = { ...page, Rows: page.Rows.filter((row) => row.ID !== pending.ID) };
  transport.removeSchedule("sch_nightly");
  await waitFor(() => expect(control.isConnected).toBe(false));
  expect(document.activeElement).not.toBe(unrelated);
  expect(commands(call)).toHaveLength(1);
});

it("closes a selected pin into its catalog result while Escape and Delete keep their opener", async () => {
  const { settle, call } = await openMemory();
  const reply = settle.getMockImplementation()!;
  const decided = Promise.withResolvers<CallPluginResponse>();
  settle.mockImplementationOnce(() => decided.promise);
  const trigger = await openActions();
  fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(trigger));
  expect(commands(call)).toHaveLength(0);

  await openActions();
  fireEvent.click(screen.getByRole("menuitem", { name: "Unpin" }));
  await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
  const result = within(catalog())
    .getByText(`Unpin ${active.ID} · submitting…`)
    .closest("section")!;
  await waitFor(() => expect(document.activeElement).toBe(result));
  clickBlocked(within(catalog()).getByRole("button", { name: "Approve" }));
  expect(commands(call)).toHaveLength(1);
  await act(async () => decided.resolve(await reply(...settle.mock.calls[0]!)));
  await within(catalog()).findByText(`Unpin ${active.ID} · completed.`);
  await waitFor(() => expect(screen.queryByText("Reading recorded memory…")).toBeNull());
  expect(document.activeElement).toBe(result);

  await openActions();
  fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(trigger));
  await openActions();
  await waitFor(() => expect(screen.getByRole("menu").contains(document.activeElement)).toBe(true));
  fireEvent.click(screen.getByRole("menuitem", { name: "Delete" }));
  const confirmation = screen.getByRole("region", { name: "Confirm memory deletion" });
  const cancel = within(confirmation).getByRole("button", { name: "Cancel" });
  cancel.focus();
  fireEvent.click(cancel);
  await waitFor(() => expect(document.activeElement).toBe(trigger));
  expect(commands(call)).toHaveLength(1);
});

it.each(["Approve", "Reject"] as const)(
  "hands an immediately settled %s to its committed result before the row changes",
  async (name) => {
    let page = VISUAL_MEMORY_PAGE;
    const list = vi.fn(async () => ({ output: JSON.stringify(page) }));
    const { settle, call } = await openMemory(list);
    await waitFor(() => expect(list).toHaveBeenCalledTimes(2));
    settle.mockImplementationOnce(async () => {
      page = {
        ...page,
        Rows:
          name === "Reject"
            ? page.Rows.filter((row) => row.ID !== pending.ID)
            : page.Rows.map((row) =>
                row.ID === pending.ID ? { ...row, Status: "active" as const } : row,
              ),
      };
      return { output: "{}" };
    });
    const control = screen.getByRole("button", { name });
    control.focus();
    fireEvent.click(control);
    const notice = await within(catalog()).findByText(`${name} ${pending.ID} · completed.`);
    await waitFor(() => expect(screen.queryByRole("button", { name })).toBeNull());
    expect(screen.queryByText(pending.Content, { exact: true }) !== null).toBe(name === "Approve");
    expect(document.activeElement).toBe(notice.closest("section"));
    expect(commands(call)).toHaveLength(1);
  },
);

it.each(["confirmed", "refused"] as const)(
  "keeps an immediately %s menu command in its result task",
  async (outcome) => {
    const { settle, call } = await openMemory();
    if (outcome === "refused") settle.mockRejectedValueOnce(new Error("Memory changed."));
    await openActions();
    fireEvent.click(screen.getByRole("menuitem", { name: "Unpin" }));
    const notice = await within(catalog()).findByText(
      `Unpin ${active.ID} · ${outcome === "confirmed" ? "completed" : "request refused"}.`,
    );
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
    await waitFor(() => expect(screen.queryByText("Reading recorded memory…")).toBeNull());
    expect(document.activeElement).toBe(notice.closest("section"));
    expect(commands(call)).toHaveLength(1);
  },
);

it("keeps a later search when an unpinned record leaves the current filter", async () => {
  let page = VISUAL_MEMORY_PAGE;
  const list = vi.fn(async () => ({ output: JSON.stringify(page) }));
  const { settle, call } = await openMemory(list);
  await waitFor(() => expect(list).toHaveBeenCalledTimes(2));
  fireEvent.click(screen.getByRole("button", { name: "Filters" }));
  fireEvent.click(screen.getByLabelText("Pinned only", { selector: "input" }));
  expect(screen.getByRole("checkbox", { name: "Pinned only" }).getAttribute("aria-checked")).toBe(
    "true",
  );
  const decided = Promise.withResolvers<CallPluginResponse>();
  settle.mockImplementationOnce(() => decided.promise);
  const trigger = await openActions();
  fireEvent.click(screen.getByRole("menuitem", { name: "Unpin" }));
  const notice = await within(catalog()).findByText(`Unpin ${active.ID} · submitting…`);
  await waitFor(() => expect(document.activeElement).toBe(notice.closest("section")));
  const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this page" });
  search.focus();
  fireEvent.change(search, { target: { value: "cutpoint" } });
  page = {
    ...page,
    Rows: page.Rows.map((row) => (row.ID === active.ID ? { ...row, Pinned: false } : row)),
  };
  await act(async () => decided.resolve({ output: JSON.stringify({ ...active, Pinned: false }) }));
  await within(catalog()).findByText(`Unpin ${active.ID} · completed.`);
  await waitFor(() => expect(trigger.isConnected).toBe(false));
  expect(document.activeElement).toBe(search);
  expect(search.value).toBe("cutpoint");
  expect(commands(call)).toHaveLength(1);
  expect(JSON.parse(commands(call)[0]![0].input)).toEqual({
    ID: active.ID,
    ExpectedRevision: active.Revision,
    Pinned: false,
  });
});

it("requires confirmation for the captured memory and excludes another form", async () => {
  const { settle, call } = await openMemory();
  const trigger = await openActions();
  fireEvent.click(screen.getByRole("menuitem", { name: "Delete" }));
  expect(settle).not.toHaveBeenCalled();
  expect(
    call.mock.calls.filter(([request]) => request.method !== "flame.memory/list.v2"),
  ).toHaveLength(0);
  const confirmation = screen.getByRole("region", { name: "Confirm memory deletion" });
  expect(confirmation.textContent).toContain(VISUAL_MEMORY_PAGE.Rows[1]!.Content);
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "Add memory" }).disabled).toBe(true);
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "Edit" }).disabled).toBe(true);
  const cancel = within(confirmation).getByRole("button", { name: "Cancel" });
  cancel.focus();
  fireEvent.click(cancel);
  expect(settle).not.toHaveBeenCalled();
  expect(
    call.mock.calls.filter(([request]) => request.method !== "flame.memory/list.v2"),
  ).toHaveLength(0);
  expect(screen.queryByRole("region", { name: "Confirm memory deletion" })).toBeNull();
  await waitFor(() => expect(document.activeElement).toBe(trigger));
  await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
  await openActions();
  fireEvent.click(screen.getByRole("menuitem", { name: "Delete" }));
  fireEvent.click(screen.getByRole("button", { name: "Delete memory" }));
  await within(catalog()).findByText(`Delete ${active.ID} · completed.`);
  expect(screen.queryByRole("region", { name: "Confirm memory deletion" })).toBeNull();
  expect(settle).toHaveBeenCalledOnce();
  expect(settle.mock.calls[0]![0]).toMatchObject({ method: "flame.memory/delete.v2" });
  expect(JSON.parse(settle.mock.calls[0]![0].input)).toEqual({
    ID: VISUAL_MEMORY_PAGE.Rows[1]!.ID,
    ExpectedRevision: VISUAL_MEMORY_PAGE.Rows[1]!.Revision,
  });
});

it("retains a deletion target until its original command is confirmed", async () => {
  const { settle } = await openMemory();
  settle.mockRejectedValueOnce(new MutationLookupError(new Error("receipt unavailable")));
  await openActions();
  fireEvent.click(screen.getByRole("menuitem", { name: "Delete" }));
  const confirm = screen.getByRole("button", { name: "Delete memory" });
  fireEvent.click(confirm);
  await screen.findByText(/result unknown/);
  const confirmation = screen.getByRole("region", { name: "Confirm memory deletion" });
  expect(confirmation.textContent).toContain(VISUAL_MEMORY_PAGE.Rows[1]!.Content);
  expect(within(confirmation).getByText(active.ID, { exact: true })).toBeTruthy();
  clickBlocked(within(confirmation).getByRole("button", { name: "Cancel" }));
  expect(screen.getByRole("region", { name: "Confirm memory deletion" })).toBe(confirmation);
  expect(pressEscape(confirmation)).toEqual({ consumed: true, reachedOuter: false });
  const recover = within(confirmation).getByRole("button", { name: "Check original result" });
  expect(
    within(
      within(confirmation).getByRole("group", { name: "Memory deletion confirmation" }),
    ).getByText("Original command: memory-command-1"),
  ).toBeTruthy();
  expect(recover).toBe(confirm);
  expect(screen.getAllByRole("button", { name: "Check original result" })).toHaveLength(1);
  fireEvent.click(recover);
  await within(catalog()).findByText(`Delete ${active.ID} · completed.`);
  expect(screen.queryByRole("region", { name: "Confirm memory deletion" })).toBeNull();
  expect(settle.mock.calls.map(([, key]) => key)).toEqual(["memory-command-1", "memory-command-1"]);
  expect(settle.mock.calls.map(([request]) => JSON.parse(request.input))).toEqual([
    { ID: VISUAL_MEMORY_PAGE.Rows[1]!.ID, ExpectedRevision: VISUAL_MEMORY_PAGE.Rows[1]!.Revision },
    { ID: VISUAL_MEMORY_PAGE.Rows[1]!.ID, ExpectedRevision: VISUAL_MEMORY_PAGE.Rows[1]!.Revision },
  ]);
});

it("summarizes a one-page catalog without pager controls or a focus move", async () => {
  await openMemory();
  const summary = within(catalog()).getByText(
    "Page 1 · 2 recorded items · 1 pending · 1 active on this page",
  );
  expect(document.activeElement).not.toBe(summary);
  expect(screen.queryByRole("button", { name: "Next page" })).toBeNull();
});

it("refreshes the accepted page while pending controls keep focus and refuse repeats", async () => {
  const { list } = await openPagedMemory();
  fireEvent.click(screen.getByRole("button", { name: "Next page" }));
  await screen.findByText(secondPage.Rows[0]!.Content, { exact: true });
  const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this page" });
  fireEvent.change(search, { target: { value: "Older" } });
  const read = Promise.withResolvers<CallPluginResponse>();
  list.mockImplementationOnce(() => read.promise);
  const refresh = screen.getByRole("button", { name: "Refresh" });
  refresh.focus();
  fireEvent.click(refresh);
  await screen.findByText("Reading recorded memory…");
  expect(listedInputs(list).at(-1)).toMatchObject({ Scope: "project", Cursor: "memory-page-2" });
  const issued = list.mock.calls.length;
  for (const control of [
    refresh,
    screen.getByRole("button", { name: "Previous page" }),
    screen.getByRole("button", { name: "Next page" }),
  ]) {
    expect(control.hasAttribute("disabled")).toBe(false);
    expect(control.getAttribute("aria-disabled")).toBe("true");
    fireEvent.click(control);
  }
  expect(list).toHaveBeenCalledTimes(issued);
  expect(document.activeElement).toBe(refresh);
  await act(async () => read.resolve(listed(list.mock.calls.at(-1)![0])));
  await waitFor(() => expect(screen.queryByText("Reading recorded memory…")).toBeNull());
  expect(pageSummary(2)).toBeTruthy();
  expect(document.activeElement).toBe(refresh);
  expect(search.value).toBe("Older");
  expect(screen.getByText(secondPage.Rows[0]!.Content, { exact: true })).toBeTruthy();
});

it("keeps the accepted page and its cursor chain after a failed refresh", async () => {
  const { list } = await openPagedMemory();
  await turnTo("Next page", 2);
  list.mockRejectedValueOnce(new Error("Memory catalog is unavailable."));
  const refresh = screen.getByRole("button", { name: "Refresh" });
  fireEvent.click(refresh);
  await screen.findByText("Memory catalog is unavailable.");
  expect(pageSummary(2)).toBeTruthy();
  expect(screen.getByText(secondPage.Rows[0]!.Content, { exact: true })).toBeTruthy();
  fireEvent.click(refresh);
  await waitFor(() => expect(screen.queryByText("Memory catalog is unavailable.")).toBeNull());
  await waitFor(() => expect(screen.queryByText("Reading recorded memory…")).toBeNull());
  expect(listedInputs(list).slice(-2)).toEqual([
    expect.objectContaining({ Cursor: "memory-page-2" }),
    expect.objectContaining({ Cursor: "memory-page-2" }),
  ]);
  expect(pageSummary(2)).toBeTruthy();
  await turnTo("Previous page", 1);
  expect(listedInputs(list).at(-1)).toMatchObject({ Cursor: "" });
});

it("leaves a later focus choice in place when an explicit page arrives", async () => {
  const { list } = await openPagedMemory();
  const read = Promise.withResolvers<CallPluginResponse>();
  list.mockImplementationOnce(() => read.promise);
  const next = screen.getByRole("button", { name: "Next page" });
  next.focus();
  fireEvent.click(next);
  await screen.findByText("Reading recorded memory…");
  const search = screen.getByRole("searchbox", { name: "Search this page" });
  search.focus();
  await act(async () => read.resolve(listed(list.mock.calls.at(-1)![0])));
  await waitFor(() => expect(pageSummary(2)).toBeTruthy());
  expect(document.activeElement).toBe(search);
});

it("keeps first and last page bounds focusable without reading", async () => {
  const { list } = await openPagedMemory();
  const previous = screen.getByRole("button", { name: "Previous page" });
  expect(previous.hasAttribute("disabled")).toBe(false);
  expect(previous.getAttribute("aria-disabled")).toBe("true");
  previous.focus();
  expect(document.activeElement).toBe(previous);
  fireEvent.click(previous);
  expect(list).toHaveBeenCalledTimes(2);
  await turnTo("Next page", 2);
  const next = screen.getByRole("button", { name: "Next page" });
  expect(next.hasAttribute("disabled")).toBe(false);
  expect(next.getAttribute("aria-disabled")).toBe("true");
  next.focus();
  expect(document.activeElement).toBe(next);
  fireEvent.click(next);
  expect(list).toHaveBeenCalledTimes(3);
  expect(pageSummary(2)).toBeTruthy();
});

it("starts a newly selected scope at its first page", async () => {
  const { list } = await openPagedMemory();
  await turnTo("Next page", 2);
  fireEvent.click(screen.getByRole("radio", { name: "User" }));
  // Both the scope read and the re-established subscription read belong to the new scope.
  await waitFor(() =>
    expect(listedInputs(list).filter((input) => input.Scope === "user")).toHaveLength(2),
  );
  await waitFor(() => expect(screen.queryByText("Reading recorded memory…")).toBeNull());
  expect(
    listedInputs(list)
      .filter((input) => input.Scope === "user")
      .map((input) => input.Cursor),
  ).toEqual(["", ""]);
  expect(pageSummary(1)).toBeTruthy();
});

it("rereads the accepted page when the owner reports a data change", async () => {
  const { list, transport } = await openPagedMemory();
  await turnTo("Next page", 2);
  const search = screen.getByRole("searchbox", { name: "Search this page" });
  search.focus();
  const issued = list.mock.calls.length;
  // The visual Runtime publishes this generic plugins.dataChanged to every data observer.
  transport.removeSchedule("sch_nightly");
  await waitFor(() => expect(list.mock.calls.length).toBeGreaterThan(issued));
  await waitFor(() => expect(screen.queryByText("Reading recorded memory…")).toBeNull());
  expect(listedInputs(list).slice(issued)).toEqual([
    expect.objectContaining({ Cursor: "memory-page-2" }),
  ]);
  expect(pageSummary(2)).toBeTruthy();
  expect(document.activeElement).toBe(search);
});

it("retains traversal while the same target's refreshed context reestablishes its observer", async () => {
  const { list, context, rerender } = await openPagedMemory();
  await turnTo("Next page", 2);
  const release = Promise.withResolvers<void>();
  const subscribe = context.runtime.runtimeEvents.subscribe.bind(context.runtime.runtimeEvents);
  vi.spyOn(context.runtime.runtimeEvents, "subscribe").mockImplementationOnce(async (...args) => {
    const subscription = await subscribe(...args);
    await release.promise;
    return subscription;
  });
  const search = screen.getByRole("searchbox", { name: "Search this page" });
  search.focus();
  const issued = list.mock.calls.length;
  rerender(<Memory context={{ ...context }} />);
  try {
    await waitFor(() => expect(list.mock.calls.length).toBeGreaterThan(issued));
    await waitFor(() => expect(screen.queryByText("Reading recorded memory…")).toBeNull());
    expect(listedInputs(list).at(-1)).toMatchObject({ Cursor: "memory-page-2" });
    expect(pageSummary(2)).toBeTruthy();
  } finally {
    await act(async () => release.resolve());
  }
  await waitFor(() => expect(list.mock.calls.length).toBeGreaterThan(issued + 1));
  await waitFor(() => expect(screen.queryByText("Reading recorded memory…")).toBeNull());
  expect(pageSummary(2)).toBeTruthy();
  expect(document.activeElement).toBe(search);
});

it("retains the accepted page and navigation focus when a page read fails", async () => {
  const { list } = await openPagedMemory();
  const next = screen.getByRole("button", { name: "Next page" });
  list.mockRejectedValueOnce(new Error("Older memory is unavailable."));
  next.focus();
  fireEvent.click(next);
  await screen.findByText("Older memory is unavailable.");
  expect(pageSummary(1)).toBeTruthy();
  expect(screen.getByText(active.Content, { exact: true })).toBeTruthy();
  expect(document.activeElement).toBe(next);
  fireEvent.click(next);
  await waitFor(() => expect(document.activeElement).toBe(pageSummary(2)));
  expect(listedInputs(list).slice(-2)).toEqual([
    expect.objectContaining({ Cursor: "memory-page-2" }),
    expect.objectContaining({ Cursor: "memory-page-2" }),
  ]);
});

it("does not accept or focus a retired page read after the scope changes", async () => {
  const { list } = await openPagedMemory();
  const read = Promise.withResolvers<CallPluginResponse>();
  list.mockImplementationOnce(() => read.promise);
  const next = screen.getByRole("button", { name: "Next page" });
  next.focus();
  fireEvent.click(next);
  await screen.findByText("Reading recorded memory…");
  const user = screen.getByRole("radio", { name: "User" });
  user.focus();
  fireEvent.click(user);
  await waitFor(() => expect(listedInputs(list).at(-1)).toMatchObject({ Scope: "user" }));
  await waitFor(() => expect(screen.queryByText("Reading recorded memory…")).toBeNull());
  await act(async () => read.resolve({ output: JSON.stringify(secondPage) }));
  expect(pageSummary(1)).toBeTruthy();
  expect(screen.queryByText(secondPage.Rows[0]!.Content, { exact: true })).toBeNull();
  expect(document.activeElement).toBe(user);
});

it("focuses accepted page scope when its initiating pager disappears", async () => {
  const { list } = await openPagedMemory();
  await turnTo("Next page", 2);
  list.mockResolvedValueOnce({ output: JSON.stringify(VISUAL_MEMORY_PAGE) });
  await turnTo("Previous page", 1);
  expect(screen.queryByRole("button", { name: "Previous page" })).toBeNull();
  expect(screen.queryByRole("button", { name: "Next page" })).toBeNull();
  expect(screen.getByText(active.Content, { exact: true })).toBeTruthy();
});

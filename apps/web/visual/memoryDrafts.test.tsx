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
import { afterEach, expect, it, vi } from "vitest";
import { createFlameClient, MutationLookupError } from "@flame/runtime-client";

import { type CallPluginRequest, type CallPluginResponse } from "@flame/runtime-client/wire";
import { type ComponentContext } from "@flame/component-sdk";
import { Memory } from "../../../plugins/memory/component/component";
import { type MethodOutputs as MemoryOutputs } from "../../../plugins/memory/package/io.github.tangerg.flame/contracts/methods.generated";
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

async function openMemory() {
  const owner = new AbortController();
  const runtime = createFlameClient(
    createVisualRuntimeTransport(
      structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
      VISUAL_CAPABILITIES,
      async () => REVIEW_DIFF,
    ),
    { capabilities: () => VISUAL_CAPABILITIES },
  );
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
    if (request.method === "flame.memory/list.v2") return original(request, options);
    const key = `memory-command-${++count}`;
    return mutationResult((key) => settle(request, key), key);
  });
  const context: ComponentContext = {
    component: { ...VISUAL_MEMORY_BINDING, componentId: "memory" },
    binding: VISUAL_MEMORY_BINDING,
    runtime,
    workspace: { ref: { path: "/Users/visual/scope" } },
    telemetry: { read: () => ({ spans: [], metrics: [], logs: [] }), clear() {} },
    locale: "en",
    signal: owner.signal,
    track: (operation) => operation(owner.signal),
    saveFile: async () => "requested",
    registerCommand() {
      throw new Error("This fixture must not register client commands");
    },
    compose: () => null,
  };
  testClient.render(<Memory context={context} />);
  await screen.findByText(VISUAL_MEMORY_PAGE.Rows[1]!.Content, { exact: true });
  await waitFor(() => expect(screen.queryByText("Reading recorded memory…")).toBeNull());
  return { settle, call };
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
  await within(editor).findByText(/confirmation unknown/);
  const issued = call.mock.calls.length;
  fireEvent.click(within(catalog()).getByRole("button", { name: "Clear search and filters" }));
  expect(search.value).toBe("");
  expect(document.activeElement).toBe(search);
  expect(screen.getByRole("region", { name: "Add memory" })).toBe(editor);
  expect(content.value).toBe("Keep my unsaved memory draft.");
  expect(within(editor).getByRole<HTMLButtonElement>("button", { name: "Cancel" }).disabled).toBe(
    true,
  );
  expect(call.mock.calls).toHaveLength(issued);
  const reconcile = within(editor).getByRole("button", { name: "Check original result" });
  reconcile.focus();
  fireEvent.click(reconcile);
  await waitFor(() => expect(settle).toHaveBeenCalledTimes(2));
  search.focus();
  await act(async () => recovered.resolve(await reply(...settle.mock.calls[1]!)));
  await within(catalog()).findByText("Add project memory · confirmed.");
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
  await within(editor).findByText(/Add project memory · confirmation unknown/);
  expect(pressEscape(content)).toEqual({ consumed: true, reachedOuter: false });
  expect(screen.getByRole("region", { name: "Add memory" })).toBe(editor);
  expect(content.value).toBe("Keep my unsaved memory draft.");
  fireEvent.click(within(editor).getByRole("button", { name: "Check original result" }));
  await within(catalog()).findByText("Add project memory · confirmed.");
  expect(screen.queryByRole("region", { name: "Add memory" })).toBeNull();
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
  await within(editor).findByText(`Edit ${active.ID} · refused.`);
  expect(within(editor).getByText("Memory capacity is full.")).toBeTruthy();
  expect(within(catalog()).queryByText(/refused/)).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Approve" }));
  await within(catalog()).findByText(`Approve ${pending.ID} · confirmed.`);
  expect(within(editor).queryByText(/Approve/)).toBeNull();
  expect(within(editor).getByText(`Edit ${active.ID} · refused.`)).toBeTruthy();
  expect(screen.getByRole("region", { name: "Edit memory" })).toBe(editor);
  expect(content).toHaveProperty("value", "Keep my edited content.");
  fireEvent.click(within(editor).getByRole("button", { name: "Save" }));
  await within(catalog()).findByText(`Edit ${active.ID} · confirmed.`);
  expect(screen.queryByRole("region", { name: "Edit memory" })).toBeNull();
  expect(settle.mock.calls.map(([request]) => JSON.parse(request.input))).toEqual([
    { ID: active.ID, ExpectedRevision: active.Revision, Content: "Keep my edited content." },
    { ID: pending.ID, ExpectedRevision: pending.Revision, Decision: "approve" },
    { ID: active.ID, ExpectedRevision: active.Revision, Content: "Keep my edited content." },
  ]);
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
  await within(catalog()).findByText(`Approve ${pending.ID} · refused.`);
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
  await within(catalog()).findByText("Add project memory · confirmed.");
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
  await within(catalog()).findByText(`Approve ${pending.ID} · confirmed.`);
  const admitted = call.getMockImplementation()!;
  call.mockImplementation((request, options) =>
    request.method === "flame.memory/list.v2"
      ? (Promise.reject(new Error("Memory catalog is unavailable.")) as never)
      : admitted(request, options),
  );
  fireEvent.click(screen.getByRole("radio", { name: "User" }));
  await screen.findByText("Memory catalog is unavailable.");
  expect(screen.queryByText(`Approve ${pending.ID} · confirmed.`)).toBeNull();
  settle.mockResolvedValueOnce({
    output: JSON.stringify({
      Item: { ...active, Scope: "user", Content: "Keep my unsaved memory draft." },
      Changed: true,
    } satisfies MemoryOutputs["flame.memory/add.v2"]),
  });
  const { editor } = startDraft();
  fireEvent.click(within(editor).getByRole("button", { name: "Save" }));
  await screen.findByRole("heading", { name: "Saved memory" });
  await within(catalog()).findByText("Add user memory · confirmed.");
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
        ? `Approve ${pending.ID} · confirmed.`
        : `Unpin ${active.ID} · confirmed.`,
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
  await within(catalog()).findByText("Add project memory · confirmed.");
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
  await within(catalog()).findByText(`Unpin ${active.ID} · confirmed.`);
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
    await screen.findByText(/confirmation unknown/);
    expect(content.value).toBe("Keep my unsaved memory draft.");
    expect(within(editor).getByRole<HTMLButtonElement>("button", { name: "Cancel" }).disabled).toBe(
      true,
    );
    fireEvent.click(screen.getByRole("button", { name: "Check original result" }));
    await within(catalog()).findByText(
      source === "editor"
        ? "Add project memory · confirmed."
        : `Approve ${pending.ID} · confirmed.`,
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
  await within(catalog()).findByText(`Delete ${active.ID} · confirmed.`);
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
  fireEvent.click(screen.getByRole("button", { name: "Delete memory" }));
  await screen.findByText(/confirmation unknown/);
  const confirmation = screen.getByRole("region", { name: "Confirm memory deletion" });
  expect(confirmation.textContent).toContain(VISUAL_MEMORY_PAGE.Rows[1]!.Content);
  expect(within(confirmation).getByText(active.ID, { exact: true })).toBeTruthy();
  expect(
    within(confirmation).getByRole<HTMLButtonElement>("button", { name: "Cancel" }).disabled,
  ).toBe(true);
  expect(pressEscape(confirmation)).toEqual({ consumed: true, reachedOuter: false });
  fireEvent.click(within(confirmation).getByRole("button", { name: "Check original result" }));
  await within(catalog()).findByText(`Delete ${active.ID} · confirmed.`);
  expect(screen.queryByRole("region", { name: "Confirm memory deletion" })).toBeNull();
  expect(settle.mock.calls.map(([, key]) => key)).toEqual(["memory-command-1", "memory-command-1"]);
  expect(settle.mock.calls.map(([request]) => JSON.parse(request.input))).toEqual([
    { ID: VISUAL_MEMORY_PAGE.Rows[1]!.ID, ExpectedRevision: VISUAL_MEMORY_PAGE.Rows[1]!.Revision },
    { ID: VISUAL_MEMORY_PAGE.Rows[1]!.ID, ExpectedRevision: VISUAL_MEMORY_PAGE.Rows[1]!.Revision },
  ]);
});

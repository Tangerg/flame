import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { createFlameClient, MutationLookupError } from "@flame/runtime-contract/client";
import { createMutationPromise } from "@flame/runtime-contract/client/mutation";
import type { CallPluginRequest, CallPluginResponse } from "@flame/runtime-contract/wire";
import type { ComponentContext } from "@flame/plugin-sdk";
import { Memory } from "../../../plugins/memory/src/component";
import type { MethodOutputs as MemoryOutputs } from "../../../plugins/memory/io.github.tangerg.flame/contracts/methods.generated";
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
        request.method === "flame.memory/add.v1"
          ? ({
              Item: {
                ...VISUAL_MEMORY_PAGE.Rows[1]!,
                ID: "mem_00000000000000000000000000000003",
                Content: "Keep my unsaved memory draft.",
              },
              Changed: true,
            } satisfies MemoryOutputs["flame.memory/add.v1"])
          : request.method === "flame.memory/update.v1"
            ? VISUAL_MEMORY_PAGE.Rows[1]
            : {},
      ),
    }),
  );
  const original = runtime.plugins.call;
  let count = 0;
  const call = vi.spyOn(runtime.plugins, "call").mockImplementation((request, options) => {
    if (request.method === "flame.memory/list.v1") return original(request, options);
    const key = `memory-command-${++count}`;
    return createMutationPromise((key) => settle(request, key), key);
  });
  const context: ComponentContext = {
    component: { ...VISUAL_MEMORY_BINDING, componentId: "memory" },
    binding: VISUAL_MEMORY_BINDING,
    runtime,
    workspace: { ref: { path: "/Users/visual/scope" } },
    telemetry: { read: () => ({ spans: [], metrics: [], logs: [] }), clear() {} },
    signal: owner.signal,
    track: (operation) => operation,
    compose: () => null,
  };
  render(<Memory context={context} />);
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

it("keeps one memory form and its target until explicit cancellation", async () => {
  await openMemory();
  const { add, editor, content } = startDraft();
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "Edit" }).disabled).toBe(true);
  expect(screen.getByRole("tab", { name: "User" }).getAttribute("aria-disabled")).toBe("true");
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
    await screen.findByText("Memory change saved.");
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
  await screen.findByText("Memory change saved.");
  expect(settle.mock.calls[0]![0].method).toBe("flame.memory/add.v1");
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
  await screen.findByText("Memory change saved.");
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
    "flame.memory/update.v1",
    "flame.memory/update.v1",
  ]);
  expect(settle.mock.calls.map(([request]) => JSON.parse(request.input))).toEqual([
    { ID: VISUAL_MEMORY_PAGE.Rows[1]!.ID, Pinned: false },
    { ID: VISUAL_MEMORY_PAGE.Rows[1]!.ID, Content: "Keep my edited content." },
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
    await screen.findByText(/Confirmation is unknown/);
    expect(content.value).toBe("Keep my unsaved memory draft.");
    expect(within(editor).getByRole<HTMLButtonElement>("button", { name: "Cancel" }).disabled).toBe(
      true,
    );
    fireEvent.click(screen.getByRole("button", { name: "Check original result" }));
    await screen.findByText("Memory change saved.");
    expect(settle.mock.calls.map(([, key]) => key)).toEqual([
      "memory-command-1",
      "memory-command-1",
    ]);
    expect(
      call.mock.calls.filter(([request]) => request.method !== "flame.memory/list.v1"),
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
    call.mock.calls.filter(([request]) => request.method !== "flame.memory/list.v1"),
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
    call.mock.calls.filter(([request]) => request.method !== "flame.memory/list.v1"),
  ).toHaveLength(0);
  expect(screen.queryByRole("region", { name: "Confirm memory deletion" })).toBeNull();
  await waitFor(() => expect(document.activeElement).toBe(trigger));
  await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
  await openActions();
  fireEvent.click(screen.getByRole("menuitem", { name: "Delete" }));
  fireEvent.click(screen.getByRole("button", { name: "Delete memory" }));
  await screen.findByText("Memory change saved.");
  expect(screen.queryByRole("region", { name: "Confirm memory deletion" })).toBeNull();
  expect(settle).toHaveBeenCalledOnce();
  expect(settle.mock.calls[0]![0]).toMatchObject({ method: "flame.memory/delete.v1" });
  expect(JSON.parse(settle.mock.calls[0]![0].input)).toEqual({
    ID: VISUAL_MEMORY_PAGE.Rows[1]!.ID,
  });
});

it("retains a deletion target until its original command is confirmed", async () => {
  const { settle } = await openMemory();
  settle.mockRejectedValueOnce(new MutationLookupError(new Error("receipt unavailable")));
  await openActions();
  fireEvent.click(screen.getByRole("menuitem", { name: "Delete" }));
  fireEvent.click(screen.getByRole("button", { name: "Delete memory" }));
  await screen.findByText(/Confirmation is unknown/);
  const confirmation = screen.getByRole("region", { name: "Confirm memory deletion" });
  expect(confirmation.textContent).toContain(VISUAL_MEMORY_PAGE.Rows[1]!.Content);
  expect(
    within(confirmation).getByRole<HTMLButtonElement>("button", { name: "Cancel" }).disabled,
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "Check original result" }));
  await screen.findByText("Memory change saved.");
  expect(screen.queryByRole("region", { name: "Confirm memory deletion" })).toBeNull();
  expect(settle.mock.calls.map(([, key]) => key)).toEqual(["memory-command-1", "memory-command-1"]);
  expect(settle.mock.calls.map(([request]) => JSON.parse(request.input))).toEqual([
    { ID: VISUAL_MEMORY_PAGE.Rows[1]!.ID },
    { ID: VISUAL_MEMORY_PAGE.Rows[1]!.ID },
  ]);
});

import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { type ComponentContext } from "@flame/component-sdk";
import { createFlameClient, RpcProtocolError, type MutationPromise } from "@flame/runtime-client";
import { type SkillDetail, type SkillDiscovery } from "@flame/runtime-client/wire";
import { Skills } from "../../../plugins/skills/component/component";
import { RUNTIME_AGENT_SESSION_SNAPSHOTS } from "./agentSessionSnapshots";
import { VISUAL_CAPABILITIES, VISUAL_WORKSPACE_PATH } from "./agentFixtureFacts";
import { createVisualRuntimeTransport } from "./createVisualRuntimeTransport";
import { REVIEW_DIFF } from "./diffSnapshot";
import {
  VISUAL_SKILL_DISCOVERY,
  VISUAL_SKILL_DOCUMENTS,
  VISUAL_SKILL_PROPOSALS,
} from "./skillSnapshots";
const [RISK_FIRST, RUNTIME_BOUNDARIES] = VISUAL_SKILL_DOCUMENTS as [SkillDetail, SkillDetail];

const releases: Array<() => Promise<void>> = [];

afterEach(async () => {
  cleanup();
  await Promise.all(releases.splice(0).map((release) => release()));
  vi.restoreAllMocks();
});

async function mountSkills() {
  const owner = new AbortController();
  const transport = createVisualRuntimeTransport(
    structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
    VISUAL_CAPABILITIES,
    async () => REVIEW_DIFF,
    true,
  );
  const sendRequest = transport.send.bind(transport);
  const send = vi.spyOn(transport, "send");
  const runtime = createFlameClient(transport, { capabilities: () => VISUAL_CAPABILITIES });
  const workspace = runtime.workspace({ path: VISUAL_WORKSPACE_PATH });
  const listDiscovered = vi.spyOn(workspace.skills, "listDiscovered");
  const getDiscovered = vi.spyOn(workspace.skills, "getDiscovered");
  vi.spyOn(runtime, "workspace").mockReturnValue(workspace);
  const context: ComponentContext = {
    component: { installationId: "skills", digest: "b".repeat(64), componentId: "skills" },
    runtime,
    workspace: { ref: { path: VISUAL_WORKSPACE_PATH } },
    telemetry: { read: () => ({ spans: [], metrics: [], logs: [] }), clear() {} },
    locale: "en",
    signal: owner.signal,
    track: (operation) => operation,
    compose: () => null,
  };
  const outsideEscape = vi.fn();
  const returnHandler = (event: KeyboardEvent) => {
    if (event.key === "Escape" && !event.defaultPrevented) outsideEscape();
  };
  window.addEventListener("keydown", returnHandler);
  releases.push(async () => {
    window.removeEventListener("keydown", returnHandler);
    owner.abort();
    await runtime.close();
  });
  render(
    <>
      <button>Outside action</button>
      <div data-testid="viewport" style={{ overflowY: "auto", height: 400 }}>
        <Skills context={context} />
      </div>
    </>,
  );
  // The mount read and the read that closes the subscription gap both settle first.
  await waitFor(() => expect(listDiscovered).toHaveBeenCalledTimes(2));
  await waitFor(() => expect(screen.queryByText("Reading Skills…")).toBeNull());
  return {
    listDiscovered,
    getDiscovered,
    owner,
    outsideEscape,
    runtime,
    workspace,
    send,
    sendRequest,
  };
}

const catalog = () => screen.getByRole("region", { name: "Available Skills" });

const readButtons = () => within(catalog()).getAllByRole<HTMLElement>("button");

const instructions = (name: string) =>
  screen.queryByRole("region", { name: `Instructions for ${name}` });

const back = () =>
  fireEvent.click(screen.getByRole("button", { name: "Back to available Skills" }));

async function holdRefresh(
  refresh: HTMLElement,
  read: { mock: { calls: unknown[][] } },
  call: number,
) {
  refresh.focus();
  fireEvent.click(refresh);
  await waitFor(() => expect(read).toHaveBeenCalledTimes(call));
  const signal = read.mock.calls[call - 1]!.at(-1) as AbortSignal;
  expect(document.activeElement).toBe(refresh);
  refresh.blur();
  refresh.focus();
  expect(document.activeElement).toBe(refresh);
  expect(refresh.matches(":disabled")).toBe(false);
  expect(refresh.getAttribute("aria-disabled")).toBe("true");
  fireEvent.click(refresh);
  fireEvent.keyDown(refresh, { key: "Enter" });
  fireEvent.keyDown(refresh, { key: " " });
  fireEvent.keyUp(refresh, { key: " " });
  expect(read).toHaveBeenCalledTimes(call);
  expect(signal.aborted).toBe(false);
}

it("opens a named document directly from its filtered catalog row and returns to it", async () => {
  const { listDiscovered } = await mountSkills();
  fireEvent.change(screen.getByRole("searchbox", { name: "Search this catalog" }), {
    target: { value: "runtime" },
  });
  const viewport = screen.getByTestId("viewport");
  viewport.scrollTop = 125;
  const [opener] = readButtons();
  expect(readButtons()).toHaveLength(1);
  expect(opener).toBe(
    within(catalog()).getByRole("button", {
      name: /^runtime-boundaries\s*project\s*Keep durable decisions with their Runtime owner\.$/,
    }),
  );
  expect(opener!.hasAttribute("aria-expanded")).toBe(false);
  opener!.focus();
  fireEvent.click(opener!);

  expect(document.activeElement).toBe(screen.getByRole("heading", { name: "runtime-boundaries" }));
  expect(viewport.scrollTop).toBe(0);
  expect(screen.queryByRole("searchbox")).toBeNull();
  expect(screen.queryByRole("tablist", { name: "Catalog" })).toBeNull();
  expect(screen.queryByRole("region", { name: "Available Skills" })).toBeNull();
  await waitFor(() =>
    expect(instructions("runtime-boundaries")?.textContent).toBe(RUNTIME_BOUNDARIES.instructions),
  );
  const source = screen.getByRole("button", { name: "Source and revision" });
  expect(source.getAttribute("aria-expanded")).toBe("false");
  fireEvent.click(source);
  expect(await screen.findByText(RUNTIME_BOUNDARIES.path)).toBeTruthy();

  back();
  expect(
    screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this catalog" }).value,
  ).toBe("runtime");
  expect(readButtons()).toHaveLength(1);
  expect(viewport.scrollTop).toBe(125);
  expect(document.activeElement).toBe(opener);
  expect(listDiscovered).toHaveBeenCalledTimes(2);
});

it("consumes Escape to leave a document and leaves catalog Escape to the host", async () => {
  const { outsideEscape } = await mountSkills();
  const [opener] = readButtons();
  fireEvent.click(opener!);
  const heading = screen.getByRole("heading", { name: "risk-first-review" });
  expect(document.activeElement).toBe(heading);
  fireEvent.keyDown(heading, { key: "Escape" });
  expect(outsideEscape).not.toHaveBeenCalled();
  expect(screen.queryByRole("heading", { name: "risk-first-review" })).toBeNull();
  expect(readButtons()).toHaveLength(2);
  expect(document.activeElement).toBe(opener);
  fireEvent.keyDown(opener!, { key: "Escape" });
  expect(outsideEscape).toHaveBeenCalledOnce();
  expect(readButtons()).toHaveLength(2);
});

it("names a first document failure without a retained claim, retries it and labels a retained refresh", async () => {
  const { getDiscovered } = await mountSkills();
  getDiscovered.mockRejectedValueOnce(new Error("document unavailable"));
  fireEvent.click(readButtons()[1]!);
  expect((await screen.findByRole("alert")).textContent).toBe(
    "Could not read runtime-boundaries: document unavailable",
  );
  expect(screen.getByRole("heading", { name: "runtime-boundaries" })).toBeTruthy();
  expect(screen.queryByText(/last accepted read/)).toBeNull();
  expect(screen.queryByText(/Skills in this catalog/)).toBeNull();
  expect(instructions("runtime-boundaries")).toBeNull();
  expect(screen.getByRole("button", { name: "Back to available Skills" })).toBeTruthy();

  fireEvent.click(screen.getByRole("button", { name: "Refresh document" }));
  await waitFor(() => expect(instructions("runtime-boundaries")).toBeTruthy());
  expect(screen.queryByRole("alert")).toBeNull();
  expect(getDiscovered).toHaveBeenLastCalledWith("runtime-boundaries", expect.any(AbortSignal));

  getDiscovered.mockRejectedValueOnce(new Error("owner offline"));
  fireEvent.click(screen.getByRole("button", { name: "Refresh document" }));
  expect((await screen.findByRole("alert")).textContent).toBe(
    "Could not refresh runtime-boundaries: owner offline",
  );
  expect(screen.getByText("Showing the last accepted read of runtime-boundaries.")).toBeTruthy();
  expect(instructions("runtime-boundaries")?.textContent).toBe(RUNTIME_BOUNDARIES.instructions);

  back();
  expect(screen.queryByRole("alert")).toBeNull();
  expect(readButtons()).toHaveLength(2);
});

it("keeps a pending document Refresh focusable and inert while the opened document and its return remain", async () => {
  const { listDiscovered, getDiscovered } = await mountSkills();
  const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this catalog" });
  fireEvent.change(search, { target: { value: "runtime" } });
  const [opener] = readButtons();
  opener!.focus();
  fireEvent.click(opener!);
  await waitFor(() =>
    expect(instructions("runtime-boundaries")?.textContent).toBe(RUNTIME_BOUNDARIES.instructions),
  );
  const pending = [
    Promise.withResolvers<SkillDetail>(),
    Promise.withResolvers<SkillDetail>(),
    Promise.withResolvers<SkillDetail>(),
  ];
  for (const deferred of pending) getDiscovered.mockImplementationOnce(() => deferred.promise);
  const refresh = screen.getByRole<HTMLElement>("button", { name: "Refresh document" });

  function expectOpenedDocument() {
    expect(screen.getByRole("heading", { name: "runtime-boundaries" })).toBeTruthy();
    expect(instructions("runtime-boundaries")?.textContent).toBe(RUNTIME_BOUNDARIES.instructions);
    expect(getDiscovered).toHaveBeenLastCalledWith("runtime-boundaries", expect.any(AbortSignal));
  }

  await holdRefresh(refresh, getDiscovered, 2);
  expect(screen.getByText("Reading runtime-boundaries…")).toBeTruthy();
  expectOpenedDocument();
  await act(async () => pending[0]!.resolve(RUNTIME_BOUNDARIES));
  await waitFor(() => expect(screen.queryByText("Reading runtime-boundaries…")).toBeNull());
  expect(refresh.getAttribute("aria-disabled")).not.toBe("true");
  expect(document.activeElement).toBe(refresh);
  expectOpenedDocument();

  await holdRefresh(refresh, getDiscovered, 3);
  await act(async () => pending[1]!.reject(new Error("owner offline")));
  expect((await screen.findByRole("alert")).textContent).toBe(
    "Could not refresh runtime-boundaries: owner offline",
  );
  expect(screen.getByText("Showing the last accepted read of runtime-boundaries.")).toBeTruthy();
  expect(document.activeElement).toBe(refresh);
  expectOpenedDocument();

  await holdRefresh(refresh, getDiscovered, 4);
  const outside = screen.getByRole<HTMLElement>("button", { name: "Outside action" });
  outside.focus();
  await act(async () => pending[2]!.resolve(RUNTIME_BOUNDARIES));
  await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
  expect(document.activeElement).toBe(outside);
  expectOpenedDocument();

  back();
  expect(
    screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this catalog" }).value,
  ).toBe("runtime");
  expect(readButtons()).toHaveLength(1);
  expect(document.activeElement).toBe(opener);
  expect(listDiscovered).toHaveBeenCalledTimes(2);
});

it("lets Escape from a pending document Refresh return to its catalog row and retire that read", async () => {
  const { listDiscovered, getDiscovered, outsideEscape } = await mountSkills();
  const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this catalog" });
  fireEvent.change(search, { target: { value: "runtime" } });
  const [opener] = readButtons();
  opener!.focus();
  fireEvent.click(opener!);
  await waitFor(() =>
    expect(instructions("runtime-boundaries")?.textContent).toBe(RUNTIME_BOUNDARIES.instructions),
  );
  const held = Promise.withResolvers<SkillDetail>();
  getDiscovered.mockImplementationOnce(() => held.promise);
  const refresh = screen.getByRole<HTMLElement>("button", { name: "Refresh document" });
  await holdRefresh(refresh, getDiscovered, 2);
  const signal = getDiscovered.mock.calls[1]!.at(-1) as AbortSignal;

  expect((await screen.findByRole("tooltip")).textContent).toBe("Refresh document");
  fireEvent.keyDown(refresh, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("tooltip")).toBeNull());
  expect(signal.aborted).toBe(false);
  expect(instructions("runtime-boundaries")).toBeTruthy();
  expect(outsideEscape).not.toHaveBeenCalled();

  fireEvent.keyDown(refresh, { key: "Escape" });
  expect(outsideEscape).not.toHaveBeenCalled();
  expect(signal.aborted).toBe(true);
  expect(screen.queryByRole("button", { name: "Refresh document" })).toBeNull();
  expect(instructions("runtime-boundaries")).toBeNull();
  expect(
    screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this catalog" }).value,
  ).toBe("runtime");
  expect(readButtons()).toEqual([opener]);
  expect(document.activeElement).toBe(opener);

  await act(async () => held.resolve(RUNTIME_BOUNDARIES));
  expect(screen.queryByRole("button", { name: "Refresh document" })).toBeNull();
  expect(screen.queryByText("Reading runtime-boundaries…")).toBeNull();
  expect(instructions("runtime-boundaries")).toBeNull();
  expect(readButtons()).toEqual([opener]);
  expect(document.activeElement).toBe(opener);
  expect(getDiscovered).toHaveBeenCalledTimes(2);
  expect(listDiscovered).toHaveBeenCalledTimes(2);
  expect(outsideEscape).not.toHaveBeenCalled();
});

it("keeps a pending catalog Refresh focusable and inert while its query and accepted rows remain", async () => {
  const { listDiscovered } = await mountSkills();
  const pending = [
    Promise.withResolvers<SkillDiscovery>(),
    Promise.withResolvers<SkillDiscovery>(),
    Promise.withResolvers<SkillDiscovery>(),
  ];
  for (const deferred of pending) listDiscovered.mockImplementationOnce(() => deferred.promise);
  const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this catalog" });
  fireEvent.change(search, { target: { value: "runtime" } });
  const refresh = screen.getByRole<HTMLElement>("button", { name: "Refresh" });

  function expectAcceptedQuery() {
    expect(search.value).toBe("runtime");
    expect(readButtons()).toHaveLength(1);
    expect(listDiscovered).toHaveBeenLastCalledWith(expect.any(AbortSignal));
  }

  await holdRefresh(refresh, listDiscovered, 3);
  expect(screen.getByText("Reading Skills…")).toBeTruthy();
  expectAcceptedQuery();
  await act(async () => pending[0]!.resolve(VISUAL_SKILL_DISCOVERY));
  await waitFor(() => expect(screen.queryByText("Reading Skills…")).toBeNull());
  expect(refresh.getAttribute("aria-disabled")).not.toBe("true");
  expect(document.activeElement).toBe(refresh);
  expectAcceptedQuery();

  await holdRefresh(refresh, listDiscovered, 4);
  await act(async () => pending[1]!.reject(new Error("catalog offline")));
  expect((await screen.findByRole("alert")).textContent).toBe(
    "Could not read available skills: catalog offline The last accepted read is retained.",
  );
  expect(document.activeElement).toBe(refresh);
  expectAcceptedQuery();

  await holdRefresh(refresh, listDiscovered, 5);
  search.focus();
  await act(async () => pending[2]!.resolve(VISUAL_SKILL_DISCOVERY));
  await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
  expect(document.activeElement).toBe(search);
  expectAcceptedQuery();
});

it("keeps a later focus choice and ignores document reads retired by return, a newer choice or retirement", async () => {
  const { getDiscovered, owner } = await mountSkills();
  const deferred = Promise.withResolvers<SkillDetail>();
  getDiscovered.mockImplementationOnce(() => deferred.promise);
  const [riskOpener, runtimeOpener] = readButtons();
  fireEvent.click(runtimeOpener!);
  expect(screen.getByText("Reading runtime-boundaries…")).toBeTruthy();
  expect(document.activeElement).toBe(screen.getByRole("heading", { name: "runtime-boundaries" }));
  const outside = screen.getByRole<HTMLElement>("button", { name: "Outside action" });
  outside.focus();
  await act(async () => deferred.resolve(RUNTIME_BOUNDARIES));
  expect(instructions("runtime-boundaries")).toBeTruthy();
  expect(document.activeElement).toBe(outside);

  back();
  const stale = Promise.withResolvers<SkillDetail>();
  getDiscovered.mockImplementationOnce(() => stale.promise);
  fireEvent.click(riskOpener!);
  const staleSignal = getDiscovered.mock.lastCall![1]!;
  back();
  expect(staleSignal.aborted).toBe(true);
  fireEvent.click(runtimeOpener!);
  await waitFor(() => expect(instructions("runtime-boundaries")).toBeTruthy());
  await act(async () => stale.resolve(RISK_FIRST));
  expect(screen.getByRole("heading", { name: "runtime-boundaries" })).toBeTruthy();
  expect(instructions("risk-first-review")).toBeNull();

  const retired = Promise.withResolvers<SkillDetail>();
  getDiscovered.mockImplementationOnce(() => retired.promise);
  fireEvent.click(screen.getByRole("button", { name: "Refresh document" }));
  const retiredSignal = getDiscovered.mock.lastCall![1]!;
  owner.abort();
  expect(retiredSignal.aborted).toBe(true);
  await act(async () => retired.resolve(RISK_FIRST));
  expect(instructions("risk-first-review")).toBeNull();
});

it("lets a canonical catalog read remove the opener while its document stays readable", async () => {
  const { listDiscovered } = await mountSkills();
  const canonical = Promise.withResolvers<SkillDiscovery>();
  listDiscovered.mockImplementationOnce(() => canonical.promise);
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  const opener = readButtons()[1]!;
  opener.focus();
  fireEvent.click(opener);
  const heading = screen.getByRole("heading", { name: "runtime-boundaries" });
  await waitFor(() => expect(instructions("runtime-boundaries")).toBeTruthy());

  await act(async () =>
    canonical.resolve({
      ...VISUAL_SKILL_DISCOVERY,
      skills: VISUAL_SKILL_DISCOVERY.skills.slice(0, 1),
    }),
  );
  expect(opener.isConnected).toBe(false);
  expect(document.activeElement).toBe(heading);
  expect(instructions("runtime-boundaries")?.textContent).toBe(RUNTIME_BOUNDARIES.instructions);

  back();
  expect(document.activeElement).toBe(
    screen.getByRole("searchbox", { name: "Search this catalog" }),
  );
  expect(readButtons()).toHaveLength(1);
});

it("distinguishes an unreadable catalog from a successfully empty one", async () => {
  const { runtime } = await mountSkills();
  vi.spyOn(runtime.skills, "listLibrary").mockRejectedValueOnce(new Error("library offline"));
  fireEvent.click(screen.getByRole("tab", { name: "Personal Skill library" }));
  expect((await screen.findByRole("alert")).textContent).toBe(
    "Could not read personal skill library: library offline",
  );
  expect(screen.queryByText(/last accepted read/)).toBeNull();
  expect(screen.queryByText("No Skills in this catalog.")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await screen.findByText("2 user-managed Skills");
  expect(screen.queryByRole("alert")).toBeNull();
  const library = screen.getByRole("region", { name: "Personal Skill library" });
  expect(within(library).getByRole("heading", { name: "old-format-notes" })).toBeTruthy();
  expect(library.querySelector("[aria-expanded]")).toBeNull();
  expect(screen.getByRole("button", { name: "Restore" })).toBeTruthy();
});

it("returns focus to search when a canonical read removes the focused navigation row", async () => {
  const { listDiscovered } = await mountSkills();
  const canonical = Promise.withResolvers<SkillDiscovery>();
  listDiscovered.mockImplementationOnce(() => canonical.promise);
  const opener = readButtons()[1]!;
  opener.focus();
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await act(async () =>
    canonical.resolve({
      ...VISUAL_SKILL_DISCOVERY,
      skills: VISUAL_SKILL_DISCOVERY.skills.slice(0, 1),
    }),
  );
  expect(opener.isConnected).toBe(false);
  expect(document.activeElement).toBe(
    screen.getByRole("searchbox", { name: "Search this catalog" }),
  );
});

it("cancels the inspected proposal before Escape reaches the surrounding page", async () => {
  const { outsideEscape } = await mountSkills();
  fireEvent.click(screen.getByRole("tab", { name: "Skill proposals" }));
  await screen.findByText("2 pending proposals");
  const approve = screen.getAllByRole<HTMLElement>("button", { name: "Approve" })[0]!;
  approve.focus();
  fireEvent.click(approve);
  const cancel = screen.getByRole("button", { name: "Cancel" });
  expect(screen.getByRole("region", { name: "Confirm proposal decision" })).toBeTruthy();
  expect(document.activeElement).toBe(cancel);
  fireEvent.keyDown(cancel, { key: "Escape" });
  await waitFor(() =>
    expect(screen.queryByRole("region", { name: "Confirm proposal decision" })).toBeNull(),
  );
  expect(screen.queryByRole("button", { name: "Confirm approve" })).toBeNull();
  expect(document.activeElement).toBe(approve);
  expect(outsideEscape).not.toHaveBeenCalled();
});

it("keeps an unconfirmed proposal command and its original identity across catalog navigation attempts", async () => {
  const { workspace, send, sendRequest } = await mountSkills();
  const acknowledgement = Promise.withResolvers<void>();
  send.mockImplementation(async (request, signal, options) => {
    if (request.method === "skills.proposals.approve") {
      await acknowledgement.promise;
      throw new RpcProtocolError("command response", []);
    }
    return sendRequest(request, signal, options);
  });
  const approve = vi.spyOn(workspace.skills, "approveProposal");
  fireEvent.click(screen.getByRole("tab", { name: "Skill proposals" }));
  await screen.findByText("2 pending proposals");
  fireEvent.click(screen.getAllByRole("button", { name: "Approve" })[0]!);
  fireEvent.click(screen.getByRole("button", { name: "Confirm approve" }));
  const proposal = VISUAL_SKILL_PROPOSALS.data[0]!;
  expect(approve).toHaveBeenCalledWith({
    scope: proposal.scope,
    name: proposal.name,
    revision: proposal.revision,
  });
  const original = approve.mock.results[0]!.value as MutationPromise<void>;
  const available = screen.getByRole("tab", { name: "Available Skills" });
  fireEvent.click(available);
  expect(screen.getByRole("tab", { name: "Skill proposals" }).getAttribute("aria-selected")).toBe(
    "true",
  );
  await act(async () => acknowledgement.resolve());
  await screen.findByText(/The original command outcome is not confirmed/);
  expect(screen.getByText(`Original command: ${original.idempotencyKey}`)).toBeTruthy();
  const refresh = screen.getByRole<HTMLElement>("button", { name: "Refresh" });
  expect(refresh.matches(":disabled")).toBe(true);
  expect(refresh.hasAttribute("aria-disabled")).toBe(false);
  fireEvent.click(available);
  expect(screen.getByRole("tab", { name: "Skill proposals" }).getAttribute("aria-selected")).toBe(
    "true",
  );
  expect(screen.queryByRole("region", { name: "Available Skills" })).toBeNull();

  send.mockImplementation(sendRequest);
  fireEvent.click(screen.getByRole("button", { name: "Reconcile original command" }));
  await screen.findByText(`Approve ${proposal.name} completed.`);
  expect(approve).toHaveBeenCalledOnce();
  const attempts = send.mock.calls.filter(
    ([request]) => request.method === "skills.proposals.approve",
  );
  expect(attempts).toHaveLength(2);
  expect(attempts[1]![0].params).toEqual(attempts[0]![0].params);
  expect(attempts[1]![2]?.idempotencyKey).toBe(original.idempotencyKey);
  expect(attempts[1]![2]?.idempotencyNamespace).toBe(attempts[0]![2]?.idempotencyNamespace);
  fireEvent.click(available);
  await screen.findByText("2 available Skills");
});

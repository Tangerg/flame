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

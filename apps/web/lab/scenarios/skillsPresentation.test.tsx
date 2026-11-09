import { unavailableComponentNotifications } from "@/test/unavailableNotifications";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { type ComponentContext } from "@flame/component-sdk";
import {
  createFlameClient,
  RpcError,
  RpcProtocolError,
  type MutationPromise,
} from "@flame/runtime-client";
import { type SkillDetail, type SkillDiscovery } from "@flame/runtime-client/wire";
import { Skills } from "../../../../plugins/skills/component/component";
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
  document.getSelection()?.removeAllRanges();
  cleanup();
  await Promise.all(releases.splice(0).map((release) => release()));
  vi.restoreAllMocks();
});

const LABELS = {
  en: { reading: "Reading Skills…", search: "Search this catalog", clear: "Clear search" },
  zh: { reading: "正在读取技能…", search: "搜索此目录", clear: "清除搜索" },
} as const;

async function mountSkills(locale: keyof typeof LABELS = "en") {
  const owner = new AbortController();
  const transport = createVisualRuntimeTransport(
    structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
    VISUAL_CAPABILITIES,
    async () => REVIEW_DIFF,
    { acceptSkillProposals: true },
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
  testClient.render(
    <>
      <button>Outside action</button>
      <div data-testid="viewport" style={{ overflowY: "auto", height: 400 }}>
        <Skills context={context} />
      </div>
    </>,
  );
  // The mount read and the read that closes the subscription gap both settle first.
  await waitFor(() => expect(listDiscovered).toHaveBeenCalledTimes(2));
  await waitFor(() => expect(screen.queryByText(LABELS[locale].reading)).toBeNull());
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

const internalError = (detail: string) =>
  new RpcError({
    code: -32603,
    message: "internal_error",
    data: { type: "internal_error", detail },
  });

const instructions = (name: string) =>
  screen.queryByRole("region", { name: `Instructions for ${name}` });

const summaryStatus = (summary: string) =>
  screen.findByText(summary, { selector: "[role=status]" });

function expectSummary(summary: string) {
  expect(screen.getByRole("heading", { name: summary }).textContent).toBe(summary);
  expect(screen.getByText(summary, { selector: "[role=status]" })).toBeTruthy();
}

it.each([
  {
    locale: "en",
    catalogName: "Available Skills",
    query: "risk",
    summaries: ["2 available Skills", "1 of 2 available Skills", "0 of 2 available Skills"],
  },
  {
    locale: "en",
    catalogName: "Skill proposals",
    query: "release",
    summaries: ["2 pending proposals", "1 of 2 pending proposals", "0 of 2 pending proposals"],
  },
  {
    locale: "en",
    catalogName: "Personal Skill library",
    query: "risk",
    summaries: [
      "2 user-managed Skills",
      "1 of 2 user-managed Skills",
      "0 of 2 user-managed Skills",
    ],
  },
  {
    locale: "zh",
    catalogName: "可用技能",
    query: "risk",
    summaries: ["2 个可用技能", "2 个可用技能中的 1 个", "2 个可用技能中的 0 个"],
  },
  {
    locale: "zh",
    catalogName: "技能提议",
    query: "release",
    summaries: ["2 项待处理提议", "2 项待处理提议中的 1 项", "2 项待处理提议中的 0 项"],
  },
  {
    locale: "zh",
    catalogName: "个人技能库",
    query: "risk",
    summaries: ["2 个用户管理的技能", "2 个用户管理的技能中的 1 个", "2 个用户管理的技能中的 0 个"],
  },
] as const)(
  "summarizes the $locale $catalogName result set against its accepted catalog in heading and status",
  async ({ locale, catalogName, query, summaries: [all, one, none] }) => {
    const { runtime, workspace, listDiscovered } = await mountSkills(locale);
    const listProposals = vi.spyOn(workspace.skills, "listProposals");
    const listLibrary = vi.spyOn(runtime.skills, "listLibrary");
    const radio = screen.getByRole("radio", { name: catalogName });
    if (radio.getAttribute("aria-checked") !== "true") fireEvent.click(radio);
    await summaryStatus(all);
    expect(screen.getByRole("region", { name: catalogName })).toBeTruthy();
    const reads = () =>
      [listDiscovered, listProposals, listLibrary].map((read) => read.mock.calls.length);
    const accepted = reads();
    const search = screen.getByRole<HTMLInputElement>("searchbox", { name: LABELS[locale].search });
    search.focus();

    fireEvent.change(search, { target: { value: query } });
    expectSummary(one);
    expect(document.activeElement).toBe(search);

    fireEvent.change(search, { target: { value: "not-a-skill" } });
    expectSummary(none);
    expect(document.activeElement).toBe(search);

    fireEvent.click(screen.getByRole("button", { name: LABELS[locale].clear }));
    expectSummary(all);
    await waitFor(() => expect(document.activeElement).toBe(search));
    expect(reads()).toEqual(accepted);
  },
);

it("keeps the accepted summary through pending and failed refreshes and follows canonical counts", async () => {
  const { listDiscovered } = await mountSkills();
  const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this catalog" });
  const refresh = screen.getByRole<HTMLElement>("button", { name: "Refresh" });
  fireEvent.change(search, { target: { value: "risk" } });
  expectSummary("1 of 2 available Skills");

  const pending = Promise.withResolvers<SkillDiscovery>();
  listDiscovered.mockImplementationOnce(() => pending.promise);
  fireEvent.click(refresh);
  await screen.findByText("Reading Skills…");
  expectSummary("1 of 2 available Skills");
  await act(async () => pending.reject(new Error("catalog offline")));
  expect((await screen.findByRole("alert")).textContent).toBe(
    "Could not read available skills: catalog offline The last accepted read is retained.",
  );
  expectSummary("1 of 2 available Skills");

  listDiscovered.mockResolvedValueOnce({
    ...VISUAL_SKILL_DISCOVERY,
    skills: VISUAL_SKILL_DISCOVERY.skills.slice(0, 1),
  });
  fireEvent.click(refresh);
  await summaryStatus("1 available Skill");
  expectSummary("1 available Skill");
  expect(screen.queryByRole("alert")).toBeNull();
  fireEvent.change(search, { target: { value: "runtime" } });
  expectSummary("0 of 1 available Skill");
  expect(screen.getByText("No matching Skills in this catalog.")).toBeTruthy();

  listDiscovered.mockResolvedValueOnce({ ...VISUAL_SKILL_DISCOVERY, skills: [] });
  fireEvent.click(refresh);
  await summaryStatus("0 available Skills");
  expectSummary("0 available Skills");
  expect(screen.getByText("No Skills in this catalog.")).toBeTruthy();
});

it("starts a changed catalog query at its first result and keeps search focus", async () => {
  await mountSkills();
  const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this catalog" });
  const viewport = screen.getByTestId("viewport");
  viewport.scrollTop = 280;
  search.focus();

  fireEvent.change(search, { target: { value: "runtime" } });

  expect(viewport.scrollTop).toBe(0);
  expect(readButtons()).toHaveLength(1);
  expect(readButtons()[0]!.textContent).toContain("runtime-boundaries");
  expect(document.activeElement).toBe(search);
});

it.each(["button", "Escape"] as const)(
  "reveals the catalog beginning when clearing its query with %s",
  async (method) => {
    const { outsideEscape } = await mountSkills();
    const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this catalog" });
    fireEvent.change(search, { target: { value: "runtime" } });
    const viewport = screen.getByTestId("viewport");
    viewport.scrollTop = 125;
    search.focus();

    if (method === "button") fireEvent.click(screen.getByRole("button", { name: "Clear search" }));
    else fireEvent.keyDown(search, { key: "Escape" });

    expect(viewport.scrollTop).toBe(0);
    expect(search.value).toBe("");
    expect(readButtons()).toHaveLength(2);
    expect(outsideEscape).not.toHaveBeenCalled();
    await waitFor(() => expect(document.activeElement).toBe(search));
  },
);

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
      name: /^runtime-boundaries\s*Project\s*Keep durable decisions with their Runtime owner\.$/,
    }),
  );
  expect(opener!.hasAttribute("aria-expanded")).toBe(false);
  opener!.focus();
  fireEvent.click(opener!);

  expect(document.activeElement).toBe(screen.getByRole("heading", { name: "runtime-boundaries" }));
  expect(viewport.scrollTop).toBe(0);
  expect(screen.queryByRole("searchbox")).toBeNull();
  expect(screen.queryByRole("radiogroup", { name: "Catalog" })).toBeNull();
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

function expectInstructionsSelected(name: string, text: string, outside: Element[]) {
  const selection = document.getSelection()!;
  expect(selection.rangeCount).toBe(1);
  const range = selection.getRangeAt(0);
  expect(range.toString()).toBe(text);
  expect(range.startContainer).toBe(instructions(name));
  expect(range.endContainer).toBe(instructions(name));
  for (const element of outside) expect(range.intersectsNode(element)).toBe(false);
}

const documentFraming = () => [
  screen.getByRole("button", { name: "Outside action" }),
  screen.getByRole("heading", { name: "runtime-boundaries" }),
  screen.getByRole("button", { name: "Back to available Skills" }),
  screen.getByRole("button", { name: "Refresh document" }),
  screen.getByRole("button", { name: "Source and revision" }),
  screen.getByRole("region", { name: "Available Skills", hidden: true }),
];

it.each(["metaKey", "ctrlKey"] as const)(
  "selects only the accepted instructions with %s+A from the document heading and leaves search native",
  async (modifier) => {
    document.getSelection()?.removeAllRanges();
    const { listDiscovered, getDiscovered } = await mountSkills();
    const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this catalog" });
    fireEvent.change(search, { target: { value: "runtime" } });
    const viewport = screen.getByTestId("viewport");
    viewport.scrollTop = 125;
    const [opener] = readButtons();
    opener!.focus();
    fireEvent.click(opener!);
    const heading = screen.getByRole<HTMLElement>("heading", { name: "runtime-boundaries" });
    expect(document.activeElement).toBe(heading);
    await waitFor(() =>
      expect(instructions("runtime-boundaries")?.textContent).toBe(RUNTIME_BOUNDARIES.instructions),
    );
    fireEvent.click(screen.getByRole("button", { name: "Source and revision" }));
    const path = await screen.findByText(RUNTIME_BOUNDARIES.path);
    heading.focus();
    const reads = getDiscovered.mock.calls.length;

    expect(fireEvent.keyDown(heading, { key: "a", [modifier]: true })).toBe(false);
    expectInstructionsSelected("runtime-boundaries", RUNTIME_BOUNDARIES.instructions, [
      ...documentFraming(),
      path,
    ]);
    expect(document.activeElement).toBe(heading);
    expect(getDiscovered).toHaveBeenCalledTimes(reads);
    expect(fireEvent.copy(heading)).toBe(true);
    expect(document.getSelection()!.toString()).toBe(RUNTIME_BOUNDARIES.instructions);

    fireEvent.keyDown(heading, { key: "Escape" });
    const returned = screen.getByRole<HTMLInputElement>("searchbox", {
      name: "Search this catalog",
    });
    expect(returned.value).toBe("runtime");
    expect(viewport.scrollTop).toBe(125);
    expect(document.activeElement).toBe(opener);
    document.getSelection()!.removeAllRanges();
    returned.focus();
    expect(fireEvent.keyDown(returned, { key: "a", [modifier]: true })).toBe(true);
    expect(document.getSelection()!.rangeCount).toBe(0);
    expect(document.activeElement).toBe(returned);
    expect(getDiscovered).toHaveBeenCalledTimes(reads);
    expect(listDiscovered).toHaveBeenCalledTimes(2);
  },
);

it("keeps a document Select All on the accepted instructions through a pending refresh", async () => {
  document.getSelection()?.removeAllRanges();
  const { getDiscovered } = await mountSkills();
  fireEvent.change(screen.getByRole("searchbox", { name: "Search this catalog" }), {
    target: { value: "runtime" },
  });
  const [opener] = readButtons();
  opener!.focus();
  fireEvent.click(opener!);
  const heading = screen.getByRole<HTMLElement>("heading", { name: "runtime-boundaries" });
  await waitFor(() =>
    expect(instructions("runtime-boundaries")?.textContent).toBe(RUNTIME_BOUNDARIES.instructions),
  );
  const pending = Promise.withResolvers<SkillDetail>();
  getDiscovered.mockImplementationOnce(() => pending.promise);
  fireEvent.click(screen.getByRole("button", { name: "Refresh document" }));
  await screen.findByText("Reading runtime-boundaries…");
  heading.focus();

  expect(fireEvent.keyDown(heading, { key: "a", metaKey: true })).toBe(false);
  expectInstructionsSelected(
    "runtime-boundaries",
    RUNTIME_BOUNDARIES.instructions,
    documentFraming(),
  );
  expect(document.activeElement).toBe(heading);

  const outside = screen.getByRole<HTMLElement>("button", { name: "Outside action" });
  outside.focus();
  const updated = {
    ...RUNTIME_BOUNDARIES,
    instructions: "Record every durable decision beside its Runtime owner.",
  };
  await act(async () => pending.resolve(updated));
  await waitFor(() => expect(screen.queryByText("Reading runtime-boundaries…")).toBeNull());
  expect(instructions("runtime-boundaries")?.textContent).toBe(updated.instructions);
  expectInstructionsSelected("runtime-boundaries", updated.instructions, documentFraming());
  expect(document.activeElement).toBe(outside);

  heading.focus();
  expect(fireEvent.keyDown(heading, { key: "a", ctrlKey: true })).toBe(false);
  expectInstructionsSelected("runtime-boundaries", updated.instructions, documentFraming());
  expect(document.activeElement).toBe(heading);
  expect(getDiscovered).toHaveBeenCalledTimes(2);
});

it("names a first document failure without a retained claim, retries it and labels a retained refresh", async () => {
  const { getDiscovered } = await mountSkills();
  getDiscovered.mockRejectedValueOnce(internalError("document unavailable"));
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

  getDiscovered.mockRejectedValueOnce(internalError("owner offline"));
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
  const viewport = screen.getByTestId("viewport");
  viewport.scrollTop = 125;

  function expectAcceptedQuery() {
    expect(search.value).toBe("runtime");
    expect(viewport.scrollTop).toBe(125);
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
  const first = Promise.withResolvers<never>();
  vi.spyOn(runtime.skills, "listLibrary").mockImplementationOnce(() => first.promise);
  fireEvent.click(screen.getByRole("radio", { name: "Personal Skill library" }));
  await screen.findByText("Reading Skills…");
  const unread = () => {
    expect(screen.getByRole("heading", { name: "Personal Skill library" })).toBeTruthy();
    expect(screen.getByText("No accepted read.", { selector: "[role=status]" })).toBeTruthy();
    expect(screen.queryByText(/user-managed Skill/i)).toBeNull();
  };
  unread();
  await act(async () => first.reject(new Error("library offline")));
  expect((await screen.findByRole("alert")).textContent).toBe(
    "Could not read personal skill library: library offline",
  );
  expect(screen.queryByText(/last accepted read/)).toBeNull();
  expect(screen.queryByText("No Skills in this catalog.")).toBeNull();
  unread();
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await summaryStatus("2 user-managed Skills");
  expectSummary("2 user-managed Skills");
  expect(screen.queryByRole("alert")).toBeNull();
  const library = screen.getByRole("region", { name: "Personal Skill library" });
  expect(within(library).getByRole("heading", { name: "old-format-notes" })).toBeTruthy();
  expect(library.querySelector("[aria-expanded]")).toBeNull();
  expect(screen.getByRole("button", { name: "Restore" })).toBeTruthy();
});

it("keeps personal library recovery attached to its captured row through query changes", async () => {
  const { runtime, send, sendRequest } = await mountSkills();
  const acknowledgement = Promise.withResolvers<void>();
  send.mockImplementation(async (request, signal, options) => {
    if (request.method === "skills.library.restore") {
      await acknowledgement.promise;
      throw new RpcProtocolError("command response", []);
    }
    return sendRequest(request, signal, options);
  });
  const restore = vi.spyOn(runtime.skills, "restore");
  fireEvent.click(screen.getByRole("radio", { name: "Personal Skill library" }));
  await summaryStatus("2 user-managed Skills");
  const library = screen.getByRole("region", { name: "Personal Skill library" });
  const archivedRow = () =>
    within(library).getByRole("heading", { name: "old-format-notes" }).closest("article")!;
  fireEvent.click(within(archivedRow()).getByRole("button", { name: "Restore" }));
  const original = restore.mock.results[0]!.value as MutationPromise<void>;
  await within(archivedRow()).findByRole("region", { name: "Skill command status" });

  const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this catalog" });
  search.focus();
  fireEvent.change(search, { target: { value: "risk-first" } });
  expect(within(library).queryByRole("heading", { name: "old-format-notes" })).toBeNull();
  expect(within(library).queryByRole("region", { name: "Skill command status" })).toBeNull();
  expect(screen.getAllByRole("region", { name: "Skill command status" })).toHaveLength(1);
  await act(async () => acknowledgement.resolve());
  await screen.findByText(/The original command outcome is not confirmed/);
  expect(screen.getByText(`Original command: ${original.idempotencyKey}`)).toBeTruthy();
  expect(document.activeElement).toBe(search);
  expect(
    screen.getByRole("radio", { name: "Personal Skill library" }).getAttribute("aria-checked"),
  ).toBe("true");

  fireEvent.change(search, { target: { value: "" } });
  const row = archivedRow();
  expect(within(row).getByText(`Original command: ${original.idempotencyKey}`)).toBeTruthy();
  expect(screen.getAllByRole("region", { name: "Skill command status" })).toHaveLength(1);
  send.mockImplementation(sendRequest);
  fireEvent.click(within(row).getByRole("button", { name: "Reconcile original command" }));
  const refused = 'Visual Runtime has no response for "skills.library.restore"';
  await within(row).findByText(refused);
  expect(screen.getAllByText(refused)).toHaveLength(1);
  expect(within(row).getByText("Archived")).toBeTruthy();
  expect(within(row).getByRole("button", { name: "Restore" })).toBeTruthy();
  expect(document.activeElement).toBe(search);
  expect(restore).toHaveBeenCalledExactlyOnceWith("old-format-notes");
  const attempts = send.mock.calls.filter(
    ([request]) => request.method === "skills.library.restore",
  );
  expect(attempts).toHaveLength(2);
  expect(attempts[1]![0].params).toEqual(attempts[0]![0].params);
  expect(attempts[1]![2]?.idempotencyKey).toBe(original.idempotencyKey);
  expect(attempts[1]![2]?.idempotencyNamespace).toBe(attempts[0]![2]?.idempotencyNamespace);
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
  fireEvent.click(screen.getByRole("radio", { name: "Skill proposals" }));
  await summaryStatus("2 pending proposals");
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

it.each([
  { action: "Approve", fits: true },
  { action: "Approve", fits: false },
  { action: "Reject", fits: true },
  { action: "Reject", fits: false },
] as const)(
  "reveals the whole $action review when it fits ($fits), otherwise its confirmation actions",
  async ({ action, fits }) => {
    await mountSkills();
    fireEvent.click(screen.getByRole("radio", { name: "Skill proposals" }));
    await summaryStatus("2 pending proposals");
    fireEvent.click(screen.getAllByRole("button", { name: action })[1]!);
    const form = screen.getByRole("region", { name: "Confirm proposal decision" });
    expect(
      within(form).getByText(`${action} · ${VISUAL_SKILL_PROPOSALS.data[1]!.name}`),
    ).toBeTruthy();
    const actions = within(form).getByRole("group", { name: "Proposal confirmation" });
    const cancel = within(actions).getByRole<HTMLElement>("button", { name: "Cancel" });
    const outside = screen.getByRole<HTMLElement>("button", { name: "Outside action" });

    // The frame below the sticky controls spans 250..500.
    const rectangles = new Map<Element, DOMRect>([
      [screen.getByTestId("viewport"), new DOMRect(0, 200, 800, 300)],
      [
        form.closest('[data-slot="panel"]')!.querySelector('[data-slot="panel-controls"]')!,
        new DOMRect(0, 200, 800, 50),
      ],
      [form, fits ? new DOMRect(0, 180, 800, 220) : new DOMRect(0, 100, 800, 420)],
      [actions, fits ? new DOMRect(0, 340, 800, 50) : new DOMRect(0, 470, 800, 50)],
      [cancel, fits ? new DOMRect(0, 350, 100, 32) : new DOMRect(0, 480, 100, 32)],
    ]);
    const revealed: Element[] = [];
    const nativeScroll = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "scrollIntoView");
    vi.spyOn(document.documentElement, "clientHeight", "get").mockReturnValue(600);
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (
      this: HTMLElement,
    ) {
      return rectangles.get(this) ?? new DOMRect(0, 0, 800, 600);
    });
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
      configurable: true,
      value(this: HTMLElement) {
        revealed.push(this);
      },
    });
    releases.push(async () => {
      if (nativeScroll)
        Object.defineProperty(HTMLElement.prototype, "scrollIntoView", nativeScroll);
      else Reflect.deleteProperty(HTMLElement.prototype, "scrollIntoView");
    });

    act(() => outside.focus());
    act(() => cancel.focus());
    expect(revealed).toEqual([fits ? form : actions]);
    expect(document.activeElement).toBe(cancel);
  },
);

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
  fireEvent.click(screen.getByRole("radio", { name: "Skill proposals" }));
  await summaryStatus("2 pending proposals");
  fireEvent.click(screen.getAllByRole("button", { name: "Approve" })[0]!);
  fireEvent.click(screen.getByRole("button", { name: "Confirm approve" }));
  const proposal = VISUAL_SKILL_PROPOSALS.data[0]!;
  expect(approve).toHaveBeenCalledWith({
    scope: proposal.scope,
    name: proposal.name,
    revision: proposal.revision,
  });
  const original = approve.mock.results[0]!.value as MutationPromise<void>;
  const available = screen.getByRole("radio", { name: "Available Skills" });
  fireEvent.click(available);
  expect(screen.getByRole("radio", { name: "Skill proposals" }).getAttribute("aria-checked")).toBe(
    "true",
  );
  await act(async () => acknowledgement.resolve());
  await screen.findByText(/The original command outcome is not confirmed/);
  expect(screen.getByText(`Original command: ${original.idempotencyKey}`)).toBeTruthy();
  const refresh = screen.getByRole<HTMLElement>("button", { name: "Refresh" });
  expect(refresh.matches(":disabled")).toBe(true);
  expect(refresh.hasAttribute("aria-disabled")).toBe(false);
  fireEvent.click(available);
  expect(screen.getByRole("radio", { name: "Skill proposals" }).getAttribute("aria-checked")).toBe(
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
  await summaryStatus("2 available Skills");
});

it("keeps the selected original command when two providers submit the same scoped name", async () => {
  const { workspace } = await mountSkills();
  const first = VISUAL_SKILL_PROPOSALS.data[0]!;
  const second = { ...first, revision: "f".repeat(64), instructions: "Review the second source." };
  vi.spyOn(workspace.skills, "listProposals").mockResolvedValue({
    ...VISUAL_SKILL_PROPOSALS,
    data: [first, second],
  });
  const approve = vi.spyOn(workspace.skills, "approveProposal");
  fireEvent.click(screen.getByRole("radio", { name: "Skill proposals" }));
  await summaryStatus("2 pending proposals");
  fireEvent.click(screen.getAllByRole("button", { name: "Approve" })[1]!);
  fireEvent.click(screen.getByRole("button", { name: "Confirm approve" }));
  expect(approve).toHaveBeenCalledWith({
    scope: second.scope,
    name: second.name,
    revision: second.revision,
  });
});

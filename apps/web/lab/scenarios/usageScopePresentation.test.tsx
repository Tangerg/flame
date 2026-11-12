import { unavailableComponentNotifications } from "@/test/unavailableNotifications";
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
import { type ComponentContext } from "@flame/component-sdk";
import { createFlameClient } from "@flame/runtime-client";
import { Usage } from "../../../../plugins/usage/component/component";
import { RUNTIME_AGENT_SESSION_SNAPSHOTS } from "./agentSessionSnapshots";
import { VISUAL_CAPABILITIES } from "./agentFixtureFacts";
import { createVisualRuntimeTransport } from "./createVisualRuntimeTransport";
import { VISUAL_USAGE } from "./componentSnapshots";
import { REVIEW_DIFF } from "./diffSnapshot";
const releases: Array<() => Promise<void>> = [];

afterEach(async () => {
  cleanup();
  await Promise.all(releases.splice(0).map((release) => release()));
  vi.restoreAllMocks();
});

async function openUsage({
  locale = "en",
  report,
  loaded = "Recorded usage loaded. All time.",
}: { locale?: string; report?: typeof VISUAL_USAGE; loaded?: string } = {}) {
  const owner = new AbortController();
  const runtime = createFlameClient(
    createVisualRuntimeTransport(
      structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
      VISUAL_CAPABILITIES,
      async () => REVIEW_DIFF,
    ),
    { capabilities: () => VISUAL_CAPABILITIES },
  );
  const summary = vi.spyOn(runtime.usage, "summary");
  if (report) summary.mockResolvedValueOnce(report);
  let context: ComponentContext = {
    component: { installationId: "usage-review", digest: "a".repeat(64), componentId: "usage" },
    runtime,
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
  releases.push(async () => {
    owner.abort();
    await runtime.close();
  });
  const rendered = testClient.render(<Usage context={context} />);
  function changeLocale(locale: string) {
    context = Object.freeze({ ...context, locale });
    rendered.rerender(<Usage context={context} />);
  }
  await screen.findByText(loaded);
  return { summary, context, changeLocale };
}

async function choose(name: string, label: string) {
  fireEvent.click(screen.getByRole("combobox", { name }));
  const option = await screen.findByRole("option", { name: label });
  fireEvent.pointerDown(option, { pointerType: "mouse" });
  fireEvent.click(option);
}

function attempts(key: string, calls: number) {
  return {
    key,
    total: { inputTokens: 250 * calls, outputTokens: 100 * calls, costUsd: calls / 4 },
    recordedCostUsd: calls / 4,
    coverage: {
      calls,
      reportedCalls: calls,
      pricedCalls: calls,
      startedCalls: 0,
      completedCalls: calls,
      failedCalls: 0,
      unknownCalls: 0,
    },
  };
}

const CLASSIFIED: typeof VISUAL_USAGE = {
  observedAt: "2026-10-20T00:00:00Z",
  total: { inputTokens: 1500, outputTokens: 600, costUsd: 1.5 },
  recordedCostUsd: 1.5,
  coverage: {
    calls: 6,
    reportedCalls: 6,
    pricedCalls: 6,
    startedCalls: 0,
    completedCalls: 6,
    failedCalls: 0,
    unknownCalls: 0,
  },
  byProvider: [attempts("human", 2), attempts("openai", 4)],
  byModel: [attempts("human/echo-1", 2), attempts("openai/gpt-5.6-sol", 4)],
  byDay: [attempts("2026-10-19", 6)],
  byOrigin: [
    attempts("human", 1),
    attempts("model", 3),
    attempts("automatic", 1),
    attempts("constructor", 1),
  ],
  byScope: [
    attempts("run", 2),
    attempts("session", 2),
    attempts("unattached", 1),
    attempts("__proto__", 1),
  ],
  sessions: 3,
  runs: 2,
};

function headings(name: string) {
  return within(screen.getByRole("region", { name }))
    .queryAllByRole("heading")
    .map((heading) => heading.textContent);
}

it("names known calling origins and associations without changing accounting or reading again", async () => {
  const accepted = structuredClone(CLASSIFIED);
  const { summary } = await openUsage({ report: CLASSIFIED });
  const rows = () => headings("Recorded usage by source");
  const totals = screen.getByLabelText("Recorded usage totals");
  const before = totals.textContent;
  expect(before).toContain("1.50");
  expect(before).toContain("1,500");
  expect(before).toContain("600");
  expect(rows()).toEqual(["human", "openai"]);

  await choose("Group by", "Calling origin");
  expect(rows()).toEqual(["Human", "Model", "Automatic", "constructor"]);
  expect(screen.getByText("0.75 USD")).toBeTruthy();
  expect(screen.getByText("3 attempts")).toBeTruthy();
  expect(screen.getByText("750 known input")).toBeTruthy();
  expect(screen.getByText("300 known output tokens")).toBeTruthy();
  const filter = screen.getByRole<HTMLInputElement>("searchbox", { name: "Filter sources" });
  fireEvent.change(filter, { target: { value: "AUTO" } });
  expect(rows()).toEqual(["Automatic"]);

  await choose("Group by", "Association");
  expect(screen.getByText("No matching rows.")).toBeTruthy();
  fireEvent.change(filter, { target: { value: "ONLY" } });
  expect(rows()).toEqual(["Session only"]);
  fireEvent.change(filter, { target: { value: "unattached" } });
  expect(rows()).toEqual(["Unattached"]);
  fireEvent.keyDown(filter, { key: "Escape" });
  expect(filter.value).toBe("");
  expect(rows()).toEqual(["Run", "Session only", "Unattached", "__proto__"]);

  expect(totals.textContent).toBe(before);
  expect(summary).toHaveBeenCalledOnce();
  expect(CLASSIFIED).toEqual(accepted);
});

it("finds Chinese classification labels and original keys while other identities stay raw", async () => {
  const accepted = structuredClone(CLASSIFIED);
  const { summary } = await openUsage({
    locale: "zh",
    report: CLASSIFIED,
    loaded: "已加载记录的用量。全部时间。",
  });
  const rows = () => headings("按来源记录的用量");
  const totals = screen.getByLabelText("已记录用量合计");
  const before = totals.textContent;
  const filter = screen.getByRole<HTMLInputElement>("searchbox", { name: "筛选来源" });
  fireEvent.change(filter, { target: { value: "用户" } });
  expect(screen.getByText("没有匹配的行。")).toBeTruthy();
  fireEvent.keyDown(filter, { key: "Escape" });
  expect(rows()).toEqual(["human", "openai"]);

  await choose("分组方式", "调用来源");
  expect(rows()).toEqual(["用户", "模型", "自动", "constructor"]);
  fireEvent.change(filter, { target: { value: "用户" } });
  expect(rows()).toEqual(["用户"]);
  fireEvent.change(filter, { target: { value: "Human" } });
  expect(rows()).toEqual(["用户"]);
  fireEvent.change(filter, { target: { value: "constructor" } });
  expect(rows()).toEqual(["constructor"]);

  await choose("分组方式", "关联");
  fireEvent.change(filter, { target: { value: "仅关联" } });
  expect(rows()).toEqual(["仅关联会话"]);
  fireEvent.change(filter, { target: { value: "SESSION" } });
  expect(rows()).toEqual(["仅关联会话"]);
  fireEvent.change(filter, { target: { value: "__PROTO__" } });
  expect(rows()).toEqual(["__proto__"]);
  fireEvent.keyDown(filter, { key: "Escape" });
  expect(rows()).toEqual(["运行", "仅关联会话", "未关联", "__proto__"]);

  await choose("分组方式", "模型");
  expect(rows()).toEqual(["human/echo-1", "openai/gpt-5.6-sol"]);
  expect(totals.textContent).toBe(before);
  expect(summary).toHaveBeenCalledOnce();
  expect(CLASSIFIED).toEqual(accepted);
});

function priced(key: string, costUsd?: number) {
  return {
    key,
    total: { inputTokens: 600, outputTokens: 150, ...(costUsd === undefined ? {} : { costUsd }) },
    ...(costUsd === undefined ? {} : { recordedCostUsd: costUsd }),
    coverage: {
      calls: 1,
      reportedCalls: 1,
      pricedCalls: costUsd === undefined ? 0 : 1,
      startedCalls: 0,
      completedCalls: 1,
      failedCalls: 0,
      unknownCalls: 0,
    },
  };
}

const SUB_MICRO: typeof VISUAL_USAGE = {
  observedAt: "2026-10-20T00:00:00Z",
  total: { inputTokens: 1200, outputTokens: 300, costUsd: 0.0000004 },
  recordedCostUsd: 0.0000004,
  coverage: {
    calls: 2,
    reportedCalls: 2,
    pricedCalls: 2,
    startedCalls: 0,
    completedCalls: 2,
    failedCalls: 0,
    unknownCalls: 0,
  },
  byProvider: [priced("priced-provider", 0.0000004), priced("zero-cost-provider", 0)],
  byModel: [],
  byDay: [],
  byOrigin: [],
  byScope: [],
  sessions: 1,
  runs: 1,
};

const facts = (scope: HTMLElement, term: string) =>
  within(scope)
    .getAllByText(term, { selector: "dt" })
    .map((label) => label.nextElementSibling!.textContent);

async function expand(scope: HTMLElement, name: string, revealed: string) {
  for (const trigger of within(scope).getAllByRole("button", { name })) fireEvent.click(trigger);
  await within(scope).findAllByText(revealed, { selector: "dt" });
}

it.each([
  {
    locale: "en",
    loaded: "Recorded usage loaded. All time.",
    totals: "Recorded usage totals",
    totalCost: "Total cost (USD)",
    subtotal: "Known price subtotal (USD)",
    accounting: "Accounting details",
    sources: "Recorded usage by source",
    details: "Token and coverage details",
    knownCost: "Known cost (USD)",
    tiny: "0.0000004",
    zero: "0.00",
  },
  {
    locale: "de",
    loaded: "Erfasste Nutzung geladen. Gesamter Zeitraum.",
    totals: "Summen der erfassten Nutzung",
    totalCost: "Gesamtkosten (USD)",
    subtotal: "Zwischensumme bekannter Preise (USD)",
    accounting: "Abrechnungsdetails",
    sources: "Erfasste Nutzung nach Quelle",
    details: "Token- und Abdeckungsdetails",
    knownCost: "Bekannte Kosten (USD)",
    tiny: "0,0000004",
    zero: "0,00",
  },
])(
  "keeps a sub-micro charge distinct from a recorded zero in $locale",
  async ({ locale, loaded, tiny, zero, ...label }) => {
    const accepted = structuredClone(SUB_MICRO);
    const { summary } = await openUsage({ locale, report: SUB_MICRO, loaded });
    expect(facts(screen.getByLabelText(label.totals), label.totalCost)).toEqual([tiny]);

    const accounting = screen.getByRole("button", { name: label.accounting }).closest("section")!;
    await expand(accounting, label.accounting, label.subtotal);
    expect(facts(accounting, label.subtotal)).toEqual([tiny]);
    expect(facts(accounting, label.totalCost)).toEqual([tiny]);

    const sources = screen.getByRole("region", { name: label.sources });
    expect(
      within(sources)
        .getAllByText(/ USD$/)
        .map((cost) => cost.textContent),
    ).toEqual([`${tiny} USD`, `${zero} USD`]);
    await expand(sources, label.details, label.knownCost);
    expect(facts(sources, label.knownCost)).toEqual([tiny, zero]);

    expect(summary).toHaveBeenCalledOnce();
    expect(SUB_MICRO).toEqual(accepted);
  },
);

it("keeps ordinary charges, missing prices and six significant sub-micro digits", async () => {
  const report: typeof VISUAL_USAGE = {
    ...SUB_MICRO,
    total: { inputTokens: 3000, outputTokens: 750 },
    recordedCostUsd: 3.600000123456789,
    coverage: {
      ...SUB_MICRO.coverage,
      calls: 5,
      reportedCalls: 5,
      pricedCalls: 4,
      completedCalls: 5,
    },
    byProvider: [
      priced("ordinary", 0.1),
      priced("whole", 2),
      priced("fractional", 1.5),
      priced("sub-micro", 0.000000123456789),
      priced("unpriced"),
    ],
  };
  const accepted = structuredClone(report);
  const { summary } = await openUsage({ report });
  expect(
    facts(screen.getByLabelText("Recorded usage totals"), "Known price subtotal (USD)"),
  ).toEqual(["3.60"]);

  const accounting = screen.getByRole("button", { name: "Accounting details" }).closest("section")!;
  await expand(accounting, "Accounting details", "Total cost (USD)");
  expect(facts(accounting, "Known price subtotal (USD)")).toEqual(["3.60"]);
  expect(facts(accounting, "Total cost (USD)")).toEqual(["Not reported"]);

  const sources = screen.getByRole("region", { name: "Recorded usage by source" });
  expect(
    within(sources)
      .getAllByText(/ USD$/)
      .map((cost) => cost.textContent),
  ).toEqual(["0.10 USD", "2.00 USD", "1.50 USD", "0.000000123457 USD", "Not reported USD"]);
  await expand(sources, "Token and coverage details", "Known cost (USD)");
  expect(facts(sources, "Known cost (USD)")).toEqual([
    "0.10",
    "2.00",
    "1.50",
    "0.000000123457",
    "Not reported",
  ]);

  expect(summary).toHaveBeenCalledOnce();
  expect(report).toEqual(accepted);
});

function counted(
  key: string,
  calls: number,
  inputTokens: number,
  outputTokens: number,
  pricedCalls = 0,
) {
  return {
    key,
    total: { inputTokens, outputTokens },
    ...(pricedCalls ? { recordedCostUsd: pricedCalls / 4 } : {}),
    coverage: {
      calls,
      reportedCalls: calls,
      pricedCalls,
      startedCalls: 0,
      completedCalls: calls,
      failedCalls: 0,
      unknownCalls: 0,
    },
  };
}

function countedReport(byProvider: ReturnType<typeof counted>[]): typeof VISUAL_USAGE {
  const sum = (pick: (bucket: ReturnType<typeof counted>) => number) =>
    byProvider.reduce((total, bucket) => total + pick(bucket), 0);
  const pricedCalls = sum((bucket) => bucket.coverage.pricedCalls);
  return {
    observedAt: "2026-10-20T00:00:00Z",
    total: {
      inputTokens: sum((bucket) => bucket.total.inputTokens),
      outputTokens: sum((bucket) => bucket.total.outputTokens),
    },
    ...(pricedCalls ? { recordedCostUsd: pricedCalls / 4 } : {}),
    coverage: {
      calls: sum((bucket) => bucket.coverage.calls),
      reportedCalls: sum((bucket) => bucket.coverage.reportedCalls),
      pricedCalls,
      startedCalls: 0,
      completedCalls: sum((bucket) => bucket.coverage.calls),
      failedCalls: 0,
      unknownCalls: 0,
    },
    byProvider,
    byModel: [],
    byDay: [],
    byOrigin: [],
    byScope: [],
    sessions: byProvider.length ? 1 : 0,
    runs: byProvider.length ? 1 : 0,
  };
}

const COUNTED_MANY = countedReport([
  counted("grouped", 997, 1, 1000),
  counted("single", 1, 1000, 0),
  counted("pair", 2, 0, 1, 1),
]);
const COUNTED_ONE = countedReport([counted("single", 1, 2, 1)]);
const COUNTED_NONE = countedReport([]);

it.each([
  {
    locale: "en",
    loaded: "Recorded usage loaded. All time.",
    refresh: "Refresh",
    many: "1,000 model attempts · 1,000 with usage · 1 priced",
    one: "1 model attempt · 1 with usage · 0 priced",
    none: "0 model attempts · 0 with usage · 0 priced",
    rows: [
      ["997 attempts", "1 known input", "1,000 known output tokens"],
      ["1 attempt", "1,000 known input", "0 known output tokens"],
      ["2 attempts", "0 known input", "1 known output token"],
    ],
    single: ["1 attempt", "2 known input", "1 known output token"],
  },
  {
    locale: "de",
    loaded: "Erfasste Nutzung geladen. Gesamter Zeitraum.",
    refresh: "Aktualisieren",
    many: "1.000 Modellversuche · 1.000 mit Nutzung · 1 mit Preis",
    one: "1 Modellversuch · 1 mit Nutzung · 0 mit Preis",
    none: "0 Modellversuche · 0 mit Nutzung · 0 mit Preis",
    rows: [
      ["997 Versuche", "1 bekanntes Eingabe-Token", "1.000 bekannte Ausgabe-Tokens"],
      ["1 Versuch", "1.000 bekannte Eingabe-Tokens", "0 bekannte Ausgabe-Tokens"],
      ["2 Versuche", "0 bekannte Eingabe-Tokens", "1 bekanntes Ausgabe-Token"],
    ],
    single: ["1 Versuch", "2 bekannte Eingabe-Tokens", "1 bekanntes Ausgabe-Token"],
  },
  {
    locale: "fr",
    loaded: "Utilisation enregistrée chargée. Toute la période.",
    refresh: "Actualiser",
    many: "1 000 tentatives de modèle · 1 000 avec utilisation · 1 avec un prix",
    one: "1 tentative de modèle · 1 avec utilisation · 0 avec un prix",
    none: "0 tentative de modèle · 0 avec utilisation · 0 avec un prix",
    rows: [
      ["997 tentatives", "1 d’entrée connue", "1 000 tokens de sortie connus"],
      ["1 tentative", "1 000 d’entrée connue", "0 token de sortie connu"],
      ["2 tentatives", "0 d’entrée connue", "1 token de sortie connu"],
    ],
    single: ["1 tentative", "2 d’entrée connue", "1 token de sortie connu"],
  },
  {
    locale: "es",
    loaded: "Uso registrado cargado. Todo el tiempo.",
    refresh: "Actualizar",
    many: "1000 intentos de modelo · 1000 con uso · 1 con precio",
    one: "1 intento de modelo · 1 con uso · 0 con precio",
    none: "0 intentos de modelo · 0 con uso · 0 con precio",
    rows: [
      ["997 intentos", "1 de entrada conocida", "1000 tokens de salida conocidos"],
      ["1 intento", "1000 de entrada conocida", "0 tokens de salida conocidos"],
      ["2 intentos", "0 de entrada conocida", "1 token de salida conocido"],
    ],
    single: ["1 intento", "2 de entrada conocida", "1 token de salida conocido"],
  },
])(
  "selects $locale count copy from each original count while keeping formatted values",
  async ({ locale, loaded, refresh, many, one, none, rows, single }) => {
    const literalText = { normalizer: (value: string) => value };
    const accepted = [COUNTED_MANY, COUNTED_ONE, COUNTED_NONE].map((report) =>
      structuredClone(report),
    );
    const { summary } = await openUsage({ locale, report: COUNTED_MANY, loaded });
    expect(screen.getByText(many, literalText)).toBeTruthy();
    for (const row of rows)
      for (const text of row) expect(screen.getByText(text, literalText)).toBeTruthy();

    summary.mockResolvedValueOnce(COUNTED_ONE);
    fireEvent.click(screen.getByRole("button", { name: refresh }));
    await screen.findByText(one, literalText);
    for (const text of single) expect(screen.getByText(text, literalText)).toBeTruthy();

    summary.mockResolvedValueOnce(COUNTED_NONE);
    fireEvent.click(screen.getByRole("button", { name: refresh }));
    await screen.findByText(none, literalText);

    expect(summary).toHaveBeenCalledTimes(3);
    expect([COUNTED_MANY, COUNTED_ONE, COUNTED_NONE]).toEqual(accepted);
  },
);

it("establishes the report range before local filters and preserves total meaning while filtering", async () => {
  const { summary } = await openUsage();
  const scope = screen.getByRole("group", { name: "Report scope" });
  const filter = screen.getByRole("searchbox", { name: "Filter sources" });
  expect(scope.compareDocumentPosition(filter) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  const totals = screen.getByLabelText("Recorded usage totals");
  const breakdown = screen.getByRole("region", { name: "Usage breakdown" });
  expect(within(breakdown).getByRole("searchbox", { name: "Filter sources" })).toBe(filter);
  expect(within(breakdown).getByRole("combobox", { name: "Group by" })).toBeTruthy();
  expect(totals.compareDocumentPosition(filter) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  const before = totals.textContent;
  fireEvent.change(filter, { target: { value: "anthropic" } });
  expect(totals.textContent).toBe(before);
  expect(screen.queryByRole("heading", { name: "openai" })).toBeNull();
  expect(screen.getByRole("heading", { name: "anthropic" })).toBeTruthy();
  expect(summary).toHaveBeenCalledOnce();
  await choose("Period", "Last 7 days");
  await screen.findByText("Recorded usage loaded. Last 7 days.");
  expect(summary).toHaveBeenLastCalledWith({ sinceDays: 7 }, expect.any(AbortSignal));
});

it("keeps the same local controls and later focus when a refreshed report becomes empty and returns", async () => {
  const { summary } = await openUsage();
  await choose("Group by", "Model");
  const filter = screen.getByRole<HTMLInputElement>("searchbox", { name: "Filter sources" });
  const group = screen.getByRole("combobox", { name: "Group by" });
  fireEvent.change(filter, { target: { value: "anthropic" } });
  const empty: typeof VISUAL_USAGE = {
    ...VISUAL_USAGE,
    total: { inputTokens: 0, outputTokens: 0, costUsd: 0 },
    recordedCostUsd: 0,
    coverage: {
      calls: 0,
      reportedCalls: 0,
      pricedCalls: 0,
      startedCalls: 0,
      completedCalls: 0,
      failedCalls: 0,
      unknownCalls: 0,
    },
    byProvider: [],
    byModel: [],
    byDay: [],
    byOrigin: [],
    byScope: [],
    sessions: 0,
    runs: 0,
  };
  const emptyRead = Promise.withResolvers<typeof VISUAL_USAGE>();
  summary.mockImplementationOnce(() => emptyRead.promise);
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  filter.focus();
  await act(async () => emptyRead.resolve(empty));
  await screen.findByText("No retained model attempts in this period.");
  expect(screen.getByRole("searchbox", { name: "Filter sources" })).toBe(filter);
  expect(screen.getByRole("combobox", { name: "Group by" })).toBe(group);
  expect(filter.value).toBe("anthropic");
  expect(group.textContent).toContain("Model");
  expect(document.activeElement).toBe(filter);
  expect(screen.queryByRole("region", { name: "Recorded usage by source" })).toBeNull();

  const populatedRead = Promise.withResolvers<typeof VISUAL_USAGE>();
  summary.mockImplementationOnce(() => populatedRead.promise);
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  filter.focus();
  await act(async () => populatedRead.resolve(VISUAL_USAGE));
  await screen.findByRole("heading", { name: "anthropic/claude-opus-5" });
  expect(screen.getByRole("searchbox", { name: "Filter sources" })).toBe(filter);
  expect(screen.getByRole("combobox", { name: "Group by" })).toBe(group);
  expect(filter.value).toBe("anthropic");
  expect(document.activeElement).toBe(filter);
  expect(screen.queryByRole("heading", { name: "openai/gpt-5.6-sol" })).toBeNull();
  expect(summary).toHaveBeenCalledTimes(3);
});

it("keeps the accepted period beside the requested range after failure and recovers on refresh", async () => {
  const { summary } = await openUsage();
  summary.mockRejectedValueOnce(new Error("Report read failed"));
  await choose("Period", "Last 30 days");
  await screen.findByRole("alert");
  const status = screen.getByRole("status");
  expect(status.textContent).toBe("Showing all time from the last successful read.");
  expect(status.closest('[data-slot="panel-controls"]')).not.toBeNull();
  expect(screen.getByRole("combobox", { name: "Period" }).textContent).toContain("Last 30 days");
  expect(screen.getByLabelText("Recorded usage totals").textContent).toContain("4.12");
  summary.mockResolvedValueOnce(VISUAL_USAGE);
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await screen.findByText("Recorded usage loaded. Last 30 days.");
  await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
});

it("keeps a pending Refresh focusable and inert while the accepted report, filter and grouping remain", async () => {
  const { summary } = await openUsage();
  const pending = [
    Promise.withResolvers<typeof VISUAL_USAGE>(),
    Promise.withResolvers<typeof VISUAL_USAGE>(),
    Promise.withResolvers<typeof VISUAL_USAGE>(),
  ];
  for (const deferred of pending) summary.mockImplementationOnce(() => deferred.promise);
  await choose("Group by", "Model");
  const filter = screen.getByRole<HTMLInputElement>("searchbox", { name: "Filter sources" });
  fireEvent.change(filter, { target: { value: "anthropic" } });
  const totals = screen.getByLabelText("Recorded usage totals").textContent;
  const refresh = screen.getByRole<HTMLButtonElement>("button", { name: "Refresh" });

  function expectAcceptedScope() {
    expect(screen.getByLabelText("Recorded usage totals").textContent).toBe(totals);
    expect(screen.getByRole("combobox", { name: "Period" }).textContent).toContain("All time");
    expect(screen.getByRole("combobox", { name: "Group by" }).textContent).toContain("Model");
    expect(filter.value).toBe("anthropic");
    expect(screen.getByRole("heading", { name: "anthropic/claude-opus-5" })).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "openai/gpt-5.6-sol" })).toBeNull();
  }

  async function startHeldRead(call: number) {
    refresh.focus();
    fireEvent.click(refresh);
    await waitFor(() => expect(summary).toHaveBeenCalledTimes(call));
    const [query, signal] = summary.mock.calls[call - 1]!;
    expect(query).toEqual({});
    expect(screen.getByText("Reading usage…")).toBeTruthy();
    expect(screen.getByText("Showing all time from the last successful read.")).toBeTruthy();
    expectAcceptedScope();
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
    expect(summary).toHaveBeenCalledTimes(call);
    expect(signal!.aborted).toBe(false);
  }

  await startHeldRead(2);
  await act(async () => pending[0]!.resolve(VISUAL_USAGE));
  await screen.findByText("Recorded usage loaded. All time.");
  expect(refresh.getAttribute("aria-disabled")).not.toBe("true");
  expect(document.activeElement).toBe(refresh);
  expectAcceptedScope();

  await startHeldRead(3);
  await act(async () => pending[1]!.reject(new Error("Report read failed")));
  expect((await screen.findByRole("alert")).textContent).toBe(
    "The current report could not be read: Report read failed",
  );
  expect(screen.getByText("Showing all time from the last successful read.")).toBeTruthy();
  expect(document.activeElement).toBe(refresh);
  expectAcceptedScope();

  await startHeldRead(4);
  filter.focus();
  await act(async () => pending[2]!.resolve(VISUAL_USAGE));
  await screen.findByText("Recorded usage loaded. All time.");
  expect(screen.queryByRole("alert")).toBeNull();
  expect(document.activeElement).toBe(filter);
  expectAcceptedScope();
});

it.each([
  {
    kind: "Refresh",
    query: {},
    requested: "全部时间",
    loaded: "已加载记录的用量。全部时间。",
    start: async () => fireEvent.click(screen.getByRole("button", { name: "Refresh" })),
  },
  {
    kind: "range",
    query: { sinceDays: 30 },
    requested: "最近 30 天",
    loaded: "已加载记录的用量。最近 30 天。",
    start: () => choose("Period", "Last 30 days"),
  },
])("accepts the original held $kind read after a display-locale change", async (copy) => {
  const { summary, context, changeLocale } = await openUsage({ report: CLASSIFIED });
  const pending = Promise.withResolvers<typeof VISUAL_USAGE>();
  summary.mockImplementationOnce(() => pending.promise);
  const filter = screen.getByRole<HTMLInputElement>("searchbox", { name: "Filter sources" });
  fireEvent.change(filter, { target: { value: "openai" } });
  try {
    await copy.start();
    await waitFor(() => expect(summary).toHaveBeenCalledTimes(2));
    const [query, originalSignal] = summary.mock.calls[1]!;
    expect(query).toEqual(copy.query);
    expect(originalSignal!.aborted).toBe(false);
    act(() => filter.focus());
    await act(async () => changeLocale("zh"));
    expect(summary).toHaveBeenCalledTimes(2);
    expect(originalSignal!.aborted).toBe(false);
    expect(context.signal.aborted).toBe(false);
    expect(screen.getByText("正在读取用量…")).toBeTruthy();
    expect(screen.getByText("显示上次成功读取时的 全部时间。")).toBeTruthy();
    expect(screen.getByRole("combobox", { name: "时间段" }).textContent).toContain(copy.requested);
    expect(facts(screen.getByLabelText("已记录用量合计"), "总费用 (USD)")).toEqual(["1.50"]);
    expect(screen.getByRole("searchbox", { name: "筛选来源" })).toBe(filter);
    expect(filter.value).toBe("openai");
    expect(document.activeElement).toBe(filter);
    await act(async () => pending.resolve(VISUAL_USAGE));
    await screen.findByText(copy.loaded);
    expect(facts(screen.getByLabelText("已记录用量合计"), "总费用 (USD)")).toEqual(["4.12"]);
    expect(screen.queryByRole("alert")).toBeNull();
    expect(summary.mock.calls.map(([query]) => query)).toEqual([{}, copy.query]);
    expect(originalSignal!.aborted).toBe(false);
    expect(document.activeElement).toBe(filter);
    expect(filter.value).toBe("openai");
  } finally {
    await act(async () => pending.resolve(VISUAL_USAGE));
  }
});

const usageRangeFailure = "  Requested range unavailable.\n原始原因 · 保留  ";

it.each([
  {
    failure: "literal",
    cause: new Error(usageRangeFailure),
    english: `The current report could not be read: ${usageRangeFailure}`,
    chinese: `无法读取当前报告：${usageRangeFailure}`,
  },
  {
    failure: "fallback",
    cause: undefined,
    english: "The current report could not be read: The report could not be read.",
    chinese: "无法读取当前报告：无法读取该报告。",
  },
])(
  "retains a settled $failure range failure across display-locale changes until explicit Refresh",
  async (copy) => {
    const { summary, changeLocale } = await openUsage({ report: CLASSIFIED });
    await choose("Group by", "Model");
    const group = screen.getByRole("combobox", { name: "Group by" });
    const filter = screen.getByRole<HTMLInputElement>("searchbox", { name: "Filter sources" });
    fireEvent.change(filter, { target: { value: "openai" } });
    const sources = screen.getByRole("region", { name: "Recorded usage by source" });
    const details = within(sources).getByRole("button", { name: "Token and coverage details" });
    fireEvent.click(details);
    summary.mockRejectedValueOnce(copy.cause);
    await choose("Period", "Last 30 days");
    expect((await screen.findByRole("alert")).textContent).toBe(copy.english);
    expect(summary).toHaveBeenCalledTimes(2);
    const period = screen.getByRole("combobox", { name: "Period" });
    expect(screen.getByText("Showing all time from the last successful read.")).toBeTruthy();
    expect(facts(screen.getByLabelText("Recorded usage totals"), "Total cost (USD)")).toEqual([
      "1.50",
    ]);
    summary.mockResolvedValueOnce(VISUAL_USAGE);
    act(() => filter.focus());
    await act(async () => changeLocale("zh"));
    expect(summary).toHaveBeenCalledTimes(2);
    expect(screen.getByRole("alert").textContent).toBe(copy.chinese);
    expect(screen.getByText("显示上次成功读取时的 全部时间。")).toBeTruthy();
    expect(facts(screen.getByLabelText("已记录用量合计"), "总费用 (USD)")).toEqual(["1.50"]);
    expect(screen.getByRole("combobox", { name: "时间段" })).toBe(period);
    expect(period.textContent).toContain("最近 30 天");
    expect(screen.getByRole("combobox", { name: "分组方式" })).toBe(group);
    expect(group.textContent).toContain("模型");
    expect(screen.getByRole("searchbox", { name: "筛选来源" })).toBe(filter);
    expect(filter.value).toBe("openai");
    expect(headings("按来源记录的用量")).toEqual(["openai/gpt-5.6-sol"]);
    expect(
      within(screen.getByRole("region", { name: "按来源记录的用量" })).getByRole("button", {
        name: "标记与覆盖明细",
      }),
    ).toBe(details);
    expect(details.getAttribute("aria-expanded")).toBe("true");
    expect(facts(sources, "已知费用 (USD)")).toEqual(["1.00"]);
    expect(document.activeElement).toBe(filter);
    await act(async () => changeLocale("en"));
    expect(summary).toHaveBeenCalledTimes(2);
    expect(screen.getByRole("alert").textContent).toBe(copy.english);
    expect(screen.getByText("Showing all time from the last successful read.")).toBeTruthy();
    expect(details.getAttribute("aria-expanded")).toBe("true");
    expect(document.activeElement).toBe(filter);
    const refresh = screen.getByRole("button", { name: "Refresh" });
    act(() => refresh.focus());
    fireEvent.click(refresh);
    act(() => filter.focus());
    await screen.findByText("Recorded usage loaded. Last 30 days.");
    expect(screen.queryByRole("alert")).toBeNull();
    expect(facts(screen.getByLabelText("Recorded usage totals"), "Total cost (USD)")).toEqual([
      "4.12",
    ]);
    expect(summary.mock.calls.map(([query]) => query)).toEqual([
      {},
      { sinceDays: 30 },
      { sinceDays: 30 },
    ]);
    expect(group.textContent).toContain("Model");
    expect(filter.value).toBe("openai");
    expect(details.getAttribute("aria-expanded")).toBe("true");
    expect(document.activeElement).toBe(filter);
  },
);

it("cancels a genuinely replaced range and ignores its late result after the current range wins", async () => {
  const { summary } = await openUsage({ report: CLASSIFIED });
  const thirty = Promise.withResolvers<typeof VISUAL_USAGE>();
  const seven = Promise.withResolvers<typeof VISUAL_USAGE>();
  summary.mockImplementationOnce(() => thirty.promise).mockImplementationOnce(() => seven.promise);
  try {
    await choose("Period", "Last 30 days");
    await waitFor(() => expect(summary).toHaveBeenCalledTimes(2));
    const oldSignal = summary.mock.calls[1]![1]!;
    expect(oldSignal.aborted).toBe(false);
    await choose("Period", "Last 7 days");
    await waitFor(() => expect(summary).toHaveBeenCalledTimes(3));
    const currentSignal = summary.mock.calls[2]![1]!;
    expect(oldSignal.aborted).toBe(true);
    expect(currentSignal.aborted).toBe(false);
    expect(screen.getByText("Showing all time from the last successful read.")).toBeTruthy();
    expect(facts(screen.getByLabelText("Recorded usage totals"), "Total cost (USD)")).toEqual([
      "1.50",
    ]);
    const filter = screen.getByRole<HTMLInputElement>("searchbox", { name: "Filter sources" });
    fireEvent.change(filter, { target: { value: "openai" } });
    act(() => filter.focus());
    await act(async () => seven.resolve(VISUAL_USAGE));
    await screen.findByText("Recorded usage loaded. Last 7 days.");
    await act(async () => thirty.resolve(SUB_MICRO));
    expect(screen.getByText("Recorded usage loaded. Last 7 days.")).toBeTruthy();
    expect(screen.getByRole("combobox", { name: "Period" }).textContent).toContain("Last 7 days");
    expect(facts(screen.getByLabelText("Recorded usage totals"), "Total cost (USD)")).toEqual([
      "4.12",
    ]);
    expect(summary.mock.calls.map(([query]) => query)).toEqual([
      {},
      { sinceDays: 30 },
      { sinceDays: 7 },
    ]);
    expect(screen.queryByRole("alert")).toBeNull();
    expect(currentSignal.aborted).toBe(false);
    expect(filter.value).toBe("openai");
    expect(document.activeElement).toBe(filter);
  } finally {
    await act(async () => {
      thirty.resolve(SUB_MICRO);
      seven.resolve(VISUAL_USAGE);
    });
  }
});

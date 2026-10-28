import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, expect, it, onTestFinished, vi } from "vitest";
import {
  type CapturedToolResult,
  type ComponentContext,
  type PluginComponent,
} from "@flame/component-sdk";
import * as ui from "@flame/ui";
import { ComponentBoundary } from "@/features/plugins/ui/InstalledComponent";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { GenUIResult } from "../../../plugins/genui/component/component";
import {
  Presentation,
  type AcceptedDocument,
  type UiNode,
} from "../../../plugins/genui/contracts/document";
import { GENUI_REPORT } from "./genuiReport";

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
): ComponentContext {
  return Object.freeze({
    component: Object.freeze({
      installationId: "captured-genui",
      digest: "a".repeat(64),
      componentId: "genui-result",
    }),
    locale,
    signal: new AbortController().signal,
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
  return { title: "Interactive review", state, nodes, actions, operations: [], bindings: [] };
}

function show(value: AcceptedDocument, locale = "en") {
  const context = capturedContext(JSON.stringify(value), locale);
  return { context, ...testClient.render(<GenUIResult context={context} />) };
}

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
  expect(within(panel).getByRole("dialog", { name: "Review hint" })).toBeTruthy();
  fireEvent.keyDown(within(panel).getByRole("dialog", { name: "Review hint" }), { key: "Escape" });
  expect(within(panel).queryByRole("dialog", { name: "Review hint" })).toBeNull();
});

it("filters, sorts, pages and selects rows without changing the captured data", async () => {
  const original = JSON.stringify(GENUI_REPORT);
  show(GENUI_REPORT);
  const table = screen.getByRole("table", { name: "Plugin delivery responsibilities" });
  await waitFor(() => expect(within(table).getAllByRole("row")).toHaveLength(3));
  const pagination = screen.getByRole("group", { name: "Plugin delivery responsibilities pages" });
  fireEvent.click(within(pagination).getByRole("button", { name: "Next" }));
  expect(within(table).getByRole("cell", { name: "Browser automation" })).toBeTruthy();
  fireEvent.click(within(table).getByLabelText("Select browser", { selector: "input" }));
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
  expect(screen.getByRole("button", { name: "Next" }).hasAttribute("disabled")).toBe(true);
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

it("keeps image loading and failures explicit and retries the original URL on user request", () => {
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
});

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
  "../../../plugins/genui/package/io.github.tangerg.flame/components/component.js",
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
    <GenUIResult context={capturedContext(JSON.stringify(GENUI_REPORT))} />,
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

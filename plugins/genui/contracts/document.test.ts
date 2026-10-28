// @vitest-environment happy-dom
import { Backend } from "@qimu/node";
import { ExactNumber, parseJSON, stringifyJSON } from "@qimu/sdk/json";
import { createParser } from "@openuidev/lang-core";
import {
  Renderer,
  useGetFieldValue,
  useSetFieldValue,
  type ComponentRenderProps,
} from "@openuidev/react-lang";
import { act, cleanup, fireEvent, render } from "@testing-library/react";
import { createElement } from "react";
import { afterEach, expect, it } from "vitest";
import {
  createDisplayLibrary,
  displayDefinitions,
  type DisplayProps,
} from "./catalog";
import {
  evaluateExpression,
  expressionCost,
  functionCatalog,
  operators,
  type Expression,
} from "./expression";
import {
  documentJSONSchema,
  InvalidPresentation,
  limits,
  parseDocument,
  Presentation,
  requestJSONSchema,
  safeURL,
  stateKey,
  validateField,
  type CapturedBinding,
  type UiDocument,
} from "./document";

afterEach(cleanup);

function sample(): UiDocument {
  return {
    title: "Interactive report",
    state: [
      { id: "amount", value: "4" },
      { id: "record", value: { value: "precise", "a-b": "9007199254740993" } },
    ],
    nodes: [
      {
        id: "input",
        type: "Input",
        props: {
          field: "amount",
          label: "Amount",
          type: "number",
          rules: { required: true, minimum: 0 },
        },
      },
      {
        id: "metric",
        type: "Metric",
        props: {
          label: "Double",
          value: {
            call: "multiply",
            args: [
              { call: "number", args: [{ state: "amount" }] },
              { literal: 2 },
            ],
          },
        },
      },
      {
        id: "text",
        type: "Text",
        props: { text: { state: "record", path: ["a-b"] } },
      },
    ],
    actions: [],
    operations: [],
  };
}

function library() {
  const renderers = Object.fromEntries(
    Object.keys(displayDefinitions).map((name) => [
      name,
      ({ props, renderNode }: ComponentRenderProps) =>
        createElement("div", null, renderNode(props.children)),
    ]),
  ) as Parameters<typeof createDisplayLibrary>[0];
  renderers.Input = ({
    props,
  }: ComponentRenderProps<DisplayProps["Input"]>) => {
    const get = useGetFieldValue(),
      set = useSetFieldValue();
    return createElement("input", {
      "aria-label": props.label,
      value:
        (get(undefined, stateKey(props.field)) as string | undefined) ?? "",
      onChange: (event) =>
        set(
          undefined,
          "GenUI",
          stateKey(props.field),
          (event.target as HTMLInputElement).value,
        ),
    });
  };
  renderers.Metric = ({ props }) =>
    createElement("output", { "aria-label": props.label }, String(props.value));
  renderers.Text = ({ props }) => createElement("p", null, props.text);
  renderers.If = ({ props, renderNode }) =>
    props.when ? renderNode(props.children) : null;
  return createDisplayLibrary(renderers);
}

function binding(kind: "query" | "command", auto = false) {
  const request = {
    id: "load",
    installationId: "plugin_1",
    contribution: "facts",
    method: "read",
    input: { object: { amount: { state: "amount" } } } as Expression,
    auto,
  };
  const captured: CapturedBinding = {
    id: "load",
    binding: {
      installationId: "plugin_1",
      digest: "sha256:captured",
      contribution: "facts",
      method: "read",
      target: { scope: "installation", reference: "plugin_1" },
      contract: stringifyJSON({
        reference: "example.facts",
        composition: "callable",
        scope: "installation",
        methods: [
          {
            name: "read",
            kind,
            target: "installation",
            input: { type: "object" },
            output: { type: "object" },
          },
        ],
      }),
    },
  };
  return { request, captured };
}

it("publishes SDK-compatible request and accepted-result schemas", async () => {
  const backend = new Backend({ name: "genui-test", version: "1.0.0" });
  backend.tool(
    {
      name: "ui_present",
      inputSchema: requestJSONSchema,
      outputSchema: documentJSONSchema,
    },
    () => ({ content: [] }),
  );
  await backend.close();
  expect(parseDocument(sample())).toEqual(sample());
  expect(() => parseDocument({ ...sample(), bindings: [] })).toThrow(
    InvalidPresentation,
  );
  expect(functionCatalog().map((item) => item.name)).toEqual(
    Object.keys(operators),
  );
  expect(
    functionCatalog().find((item) => item.name === "multiply")?.parameters,
  ).toMatchObject({ type: "array", minItems: 2, maxItems: 2 });
});

it("serializes the complete JSON owner through the public parser with no queries or mutations", () => {
  const presentation = Presentation.accept(sample());
  const display = library();
  const parsed = createParser(display.toJSONSchema()).parse(
    presentation.openUI(display),
  );
  expect(parsed.root?.typeName).toBe("Document");
  expect(parsed.meta.incomplete).toBe(false);
  expect(parsed.meta.errors).toEqual([]);
  expect(parsed.meta.unresolved).toEqual([]);
  expect(parsed.queryStatements).toEqual([]);
  expect(parsed.mutationStatements).toEqual([]);
  expect(Presentation.read(presentation.json()).snapshot()).toEqual({
    ...sample(),
    bindings: [],
  });
});

it("renders initial expressions and updates controls and computations in the same public store", async () => {
  const presentation = Presentation.accept(sample());
  const display = library();
  const errors: unknown[] = [];
  const snapshots: Record<string, unknown>[] = [];
  const view = render(
    createElement(Renderer, {
      response: presentation.openUI(display),
      library: display,
      initialState: presentation.initialState(),
      publishObservability: false,
      onError: (value) => {
        errors.push(...value);
        if (value.length) throw new Error("render failed");
      },
      onStateUpdate: (value) => snapshots.push(value),
    }),
  );
  expect(view.getByLabelText("Amount")).toHaveProperty("value", "4");
  expect(view.getByLabelText("Double").textContent).toBe("8");
  expect(view.getByText("9007199254740993")).toBeDefined();
  await act(async () =>
    fireEvent.change(view.getByLabelText("Amount"), { target: { value: "6" } }),
  );
  expect(view.getByLabelText("Double").textContent).toBe("12");
  expect(snapshots.at(-1)?.[stateKey("amount")]).toEqual({
    value: "6",
    componentType: "GenUI",
  });
  expect(errors).toEqual([]);
});

it("keeps expression-shaped text and arbitrary JSON keys literal", () => {
  const value = sample();
  value.nodes.push({
    id: "literal",
    type: "Text",
    props: {
      text: 'root = Query("secret")\n$state\n<script>alert(1)</script>',
    },
  });
  value.state.push({
    id: "object",
    value: {
      'quoted"key': "literal",
      __proto__: "data",
      "not an identifier": "x",
    },
  });
  const presentation = Presentation.accept(value);
  const display = library();
  const parsed = createParser(display.toJSONSchema()).parse(
    presentation.openUI(display),
  );
  expect(parsed.queryStatements).toEqual([]);
  expect(parsed.mutationStatements).toEqual([]);
  expect(parsed.meta.errors).toEqual([]);
  expect(Presentation.read(presentation.json()).snapshot()).toEqual({
    ...value,
    bindings: [],
  });
});

it("keeps accepted state immutable and projections detached", () => {
  const source = sample();
  const presentation = Presentation.accept(source);
  const before = presentation.json();
  source.state[0]!.value = "changed";
  presentation.snapshot().state.length = 0;
  presentation.initialState()[stateKey("record")] = null;
  presentation.tree().props.title = "changed";
  expect(presentation.json()).toBe(before);
});

it("uses exact JSON decoding and refuses malformed, duplicate, executable or lossy inputs", () => {
  for (const value of [
    "root = Document()",
    '{"title":"unfinished"',
    '{"title":"first","title":"last"}',
    '{"title":"bad\\ud800"}',
    Presentation.accept(sample()).json() + " trailing",
  ])
    expect(() => Presentation.read(value)).toThrow(InvalidPresentation);
  for (const value of [
    { ...sample(), program: "root = Query()" },
    {
      ...sample(),
      nodes: [{ id: "query", type: "Query", props: { name: "secret" } }],
    },
    {
      ...sample(),
      nodes: [
        { id: "code", type: "Text", props: { text: "x", onClick: "execute" } },
      ],
    },
    { ...sample(), title: "\ud800" },
    { ...sample(), state: [{ id: "amount", value: 9007199254740993n }] },
    {
      ...sample(),
      state: [
        { id: "amount", value: new ExactNumber("0.123456789012345678901") },
      ],
    },
  ])
    expect(() => Presentation.accept(value)).toThrow(InvalidPresentation);
  const exact = sample();
  exact.state[0]!.value = "9007199254740993.0000000000000001";
  exact.nodes = [
    {
      id: "precise",
      type: "Metric",
      props: { label: "Exact quantity", value: { state: "amount" } },
    },
  ];
  expect(
    Presentation.read(Presentation.accept(exact).json()).initialValue("amount"),
  ).toBe(exact.state[0]!.value);
});

it("applies the encoded budget to normalized content rather than transport indentation", () => {
  const value: UiDocument = {
    title: "Whitespace",
    state: [],
    actions: [],
    operations: [],
    nodes: [],
  };
  for (let index = 0; index < 5; index++)
    value.nodes.push({
      id: "text" + index,
      type: "Text",
      props: { text: "\u0001".repeat(8500) },
    });
  value.nodes.push({ id: "tail", type: "Text", props: { text: "" } });
  const base = Presentation.accept(value);
  const room = limits.bytes - new TextEncoder().encode(base.json()).length;
  value.nodes.at(-1)!.props.text = "x".repeat(room - 1);
  const presentation = Presentation.accept(value);
  const formatted = JSON.stringify(parseJSON(presentation.json()), null, 2);
  expect(new TextEncoder().encode(formatted).length).toBeGreaterThan(
    limits.bytes,
  );
  expect(Presentation.read(formatted).json()).toBe(presentation.json());
  value.nodes.at(-1)!.props.text += "xx";
  expect(() => Presentation.accept(value)).toThrow("encoded content capacity");
});

it("closes writable state, action, loop and operation references", () => {
  for (const patch of [
    {
      nodes: [
        { id: "text", type: "Text", props: { text: { state: "missing" } } },
      ],
    },
    {
      nodes: [
        {
          id: "input",
          type: "Input",
          props: { field: "$operation_load", label: "Forbidden" },
        },
      ],
    },
    { nodes: [{ id: "text", type: "Text", props: { text: { item: "row" } } }] },
    {
      actions: [
        {
          id: "bad",
          steps: [{ type: "set", state: "missing", value: { literal: "x" } }],
        },
      ],
    },
    {
      actions: [
        { id: "bad", steps: [{ type: "operation", operation: "missing" }] },
      ],
    },
    {
      state: [
        { id: "amount", value: "4" },
        { id: "amount", value: "5" },
      ],
    },
  ])
    expect(() => Presentation.accept({ ...sample(), ...patch })).toThrow(
      InvalidPresentation,
    );
});

it("captures exactly the declared real bindings and prohibits automatic commands", () => {
  const query = binding("query", true);
  const document = { ...sample(), operations: [query.request] };
  const presentation = Presentation.accept(document, [query.captured]);
  expect(presentation.operation("load")).toEqual({
    request: query.request,
    binding: query.captured.binding,
  });
  expect(() => Presentation.accept(document)).toThrow("correspond exactly");
  expect(() =>
    Presentation.accept(document, [
      {
        ...query.captured,
        binding: { ...query.captured.binding, method: "other" },
      },
    ]),
  ).toThrow("differs");
  const command = binding("command", true);
  expect(() =>
    Presentation.accept({ ...sample(), operations: [command.request] }, [
      command.captured,
    ]),
  ).toThrow("automatic operations");
  const forged = presentation.snapshot();
  forged.bindings[0]!.binding.target.reference = "other";
  expect(() => Presentation.read(stringifyJSON(forged))).toThrow(
    "target differs",
  );
});

it("preserves operator grouping, exact equality and pure collection scopes", () => {
  const evaluate = (expression: Expression) =>
    evaluateExpression(
      expression,
      () => undefined,
      () => undefined,
    );
  expect(
    evaluate({
      call: "subtract",
      args: [
        { literal: 10 },
        { call: "subtract", args: [{ literal: 4 }, { literal: 1 }] },
      ],
    }),
  ).toBe(7);
  expect(
    evaluate({
      call: "equal",
      args: [{ literal: { a: 1, b: 2 } }, { literal: { b: 2, a: 1 } }],
    }),
  ).toBe(true);
  expect(
    evaluate({
      map: {
        items: { literal: [1, 2, 3] },
        as: "row",
        limit: 3,
        value: { call: "multiply", args: [{ item: "row" }, { literal: 2 }] },
      },
    }),
  ).toEqual([2, 4, 6]);
  expect(
    evaluate({
      filter: {
        items: { literal: [1, 2, 3] },
        as: "row",
        limit: 3,
        where: { call: "greater", args: [{ item: "row" }, { literal: 1 }] },
      },
    }),
  ).toEqual([2, 3]);
  expect(
    evaluate({
      groupBy: {
        items: {
          literal: [
            { group: "a", value: 1 },
            { group: "a", value: 2 },
          ],
        },
        as: "row",
        limit: 2,
        key: { item: "row", path: ["group"] },
      },
    }),
  ).toEqual({
    a: [
      { group: "a", value: 1 },
      { group: "a", value: 2 },
    ],
  });
  expect(
    evaluate({ call: "number", args: [{ literal: "9007199254740993" }] }),
  ).toBe(null);
  expect(() =>
    evaluate({
      call: "get",
      args: [{ literal: {} }, { literal: ["constructor"] }],
    }),
  ).toThrow(InvalidPresentation);
});

it("bounds total iterations, value expansion and string construction", () => {
  const nested: Expression = {
    map: {
      items: { call: "range", args: [{ literal: 2048 }] },
      as: "outer",
      limit: 2048,
      value: {
        map: {
          items: { call: "range", args: [{ literal: 2048 }] },
          as: "inner",
          limit: 2048,
          value: { item: "inner" },
        },
      },
    },
  };
  expect(expressionCost(nested)).toBeGreaterThan(limits.evaluationSteps);
  expect(() =>
    evaluateExpression(
      nested,
      () => null,
      () => null,
    ),
  ).toThrow("evaluation budget");
  expect(() =>
    evaluateExpression(
      {
        map: {
          items: { literal: [1, 2] },
          as: "x",
          limit: 1,
          value: { item: "x" },
        },
      },
      () => null,
      () => null,
    ),
  ).toThrow("iteration exceeds");
  expect(() =>
    evaluateExpression(
      {
        call: "join",
        args: [
          { literal: Array.from({ length: 100 }, () => "a".repeat(1000)) },
          { literal: "" },
        ],
      },
      () => null,
      () => null,
    ),
  ).toThrow("capacity");
  expect(() =>
    Presentation.accept({
      ...sample(),
      nodes: [
        {
          id: "outer",
          type: "Each",
          props: { items: { literal: [] }, as: "x", limit: 1024 },
          children: [
            {
              id: "inner",
              type: "Each",
              props: { items: { literal: [] }, as: "y", limit: 1024 },
              children: [{ id: "text", type: "Text", props: { text: "x" } }],
            },
          ],
        },
      ],
    }),
  ).toThrow("expansion");
});

it("shares field validation and local-state constraints with actions", () => {
  const value = sample();
  value.nodes = [
    {
      id: "form",
      type: "Form",
      props: { action: "submit", submitLabel: "Submit" },
      children: value.nodes,
    },
  ];
  value.actions = [
    { id: "submit", steps: [{ type: "copy", text: { state: "amount" } }] },
  ];
  const presentation = Presentation.accept(value);
  expect(presentation.validateAction("submit", () => "-")).toEqual([
    { field: "amount", issue: { code: "number" } },
  ]);
  expect(validateField("", { required: true })).toEqual({ code: "required" });
  expect(validateField("2026-02-30", {}, "date")).toEqual({ code: "date" });
  expect(validateField("-", {}, "number")).toEqual({ code: "number" });
  expect(validateField("4.5", { minimum: 4 }, "number")).toBeUndefined();
  expect(presentation.validateState("amount", "-")).toBe("-");
  expect(() => presentation.validateState("amount", 4)).toThrow("state shape");
});

it("enforces safe URLs, chart/table identities and SVG placement", () => {
  for (const url of [
    "javascript:alert(1)",
    "https://user:password@example.com",
    "https://example.com/%0aevil",
    "data:text/html,x",
    " https://example.com",
  ])
    expect(() => safeURL(url)).toThrow(InvalidPresentation);
  expect(safeURL("mailto:team@example.com")).toBe("mailto:team@example.com");
  expect(() => safeURL("http://example.com/image.png", "image")).toThrow(
    InvalidPresentation,
  );
  for (const node of [
    { id: "circle", type: "Circle", props: { cx: 0, cy: 0, r: 10 } },
    {
      id: "chart",
      type: "Chart",
      props: {
        kind: "pie",
        caption: "Invalid pie",
        labels: ["One"],
        series: [{ label: "Values", values: [-1] }],
      },
    },
    {
      id: "table",
      type: "Table",
      props: {
        caption: "Invalid IDs",
        columns: [{ key: "id", label: "ID" }],
        rowKey: "id",
        rows: [{ id: "same" }, { id: "same" }],
      },
    },
  ])
    expect(() => Presentation.accept({ ...sample(), nodes: [node] })).toThrow(
      InvalidPresentation,
    );
});

it("preserves specific admission causes without swallowing programmer exceptions", () => {
  let error: unknown;
  try {
    Presentation.accept({});
  } catch (cause) {
    error = cause;
  }
  expect(error).toBeInstanceOf(InvalidPresentation);
  expect((error as Error).cause).toBeInstanceOf(Error);
  const unexpected = new Error("unexpected getter failure");
  expect(() =>
    Presentation.accept({
      get title() {
        throw unexpected;
      },
    }),
  ).toThrow(unexpected);
});

it("renders bounded Each templates with the public evaluator and scoped conditions", () => {
  const value: UiDocument = {
    title: "Rows",
    state: [
      {
        id: "rows",
        value: [
          { label: "Shown", visible: true },
          { label: "Hidden", visible: false },
        ],
      },
    ],
    actions: [],
    operations: [],
    nodes: [
      {
        id: "rows",
        type: "Each",
        props: { items: { state: "rows" }, as: "row", limit: 2 },
        children: [
          {
            id: "visible",
            type: "If",
            props: { when: { item: "row", path: ["visible"] } },
            children: [
              {
                id: "label",
                type: "Text",
                props: { text: { item: "row", path: ["label"] } },
              },
            ],
          },
        ],
      },
    ],
  };
  const presentation = Presentation.accept(value);
  const display = library();
  const errors: unknown[] = [];
  const view = render(
    createElement(Renderer, {
      response: presentation.openUI(display),
      library: display,
      initialState: presentation.initialState(),
      publishObservability: false,
      onError: (value) => errors.push(...value),
    }),
  );
  expect(view.getByText("Shown")).toBeDefined();
  expect(view.queryByText("Hidden")).toBe(null);
  expect(errors).toEqual([]);
});

it("keeps hidden form fields out of submission while resetting every declared form state", () => {
  const value: UiDocument = {
    title: "Scoped form",
    state: [
      { id: "rows", value: [] },
      { id: "required", value: "" },
    ],
    operations: [],
    actions: [
      { id: "submit", steps: [{ type: "copy", text: { literal: "done" } }] },
    ],
    nodes: [
      {
        id: "form",
        type: "Form",
        props: { action: "submit", submitLabel: "Submit" },
        children: [
          {
            id: "rows",
            type: "Each",
            props: { items: { state: "rows" }, as: "row", limit: 2 },
            children: [
              {
                id: "visible",
                type: "If",
                props: { when: { item: "row", path: ["visible"] } },
                children: [
                  {
                    id: "required",
                    type: "Input",
                    props: {
                      field: "required",
                      label: "Required",
                      rules: { required: true },
                    },
                  },
                ],
              },
            ],
          },
        ],
      },
    ],
  };
  const presentation = Presentation.accept(value);
  expect(presentation.formStateIds("submit")).toEqual(["required"]);
  expect(presentation.validateAction("submit", () => undefined)).toEqual([]);
  expect(
    presentation.validateAction("submit", (id) =>
      id === "rows" ? [{ visible: false }] : "",
    ),
  ).toEqual([]);
  expect(
    presentation.validateAction("submit", (id) =>
      id === "rows" ? [{ visible: true }] : "",
    ),
  ).toEqual([{ field: "required", issue: { code: "required" } }]);
});

it("rejects repeated timer writers and aggregate display expansion before any DOM is built", () => {
  const base: UiDocument = {
    title: "Clock",
    state: [
      { id: "seconds", value: 60 },
      { id: "running", value: false },
    ],
    actions: [],
    operations: [],
    nodes: [
      {
        id: "timer",
        type: "Timer",
        props: {
          field: "seconds",
          runningState: "running",
          label: "Countdown",
        },
      },
    ],
  };
  expect(() =>
    Presentation.accept({
      ...base,
      nodes: [...base.nodes, { ...base.nodes[0], id: "duplicate" }],
    }),
  ).toThrow("timer clock state");
  expect(() =>
    Presentation.accept({
      ...base,
      nodes: [
        {
          id: "repeat",
          type: "Each",
          props: { items: { literal: [1, 2] }, as: "item", limit: 2 },
          children: base.nodes,
        },
      ],
    }),
  ).toThrow("timer clock cannot be repeated");
  const expansion: UiDocument = {
    title: "Expansion",
    state: [{ id: "items", value: Array.from({ length: 2048 }, () => "x") }],
    actions: [],
    operations: [],
    nodes: Array.from({ length: 256 }, (_, index) => ({
      id: "list" + index,
      type: "List",
      props: { items: { state: "items" } },
    })),
  };
  expect(() => Presentation.accept(expansion)).toThrow("capacity");
});

it("calculates generic calendar, statistical, unit and pivot expressions", () => {
  const run = (call: keyof typeof operators, values: unknown[]) =>
    evaluateExpression(
      { call, args: values.map((value) => ({ literal: value as never })) },
      () => undefined,
      () => undefined,
    );
  expect(run("dateAdd", ["2026-01-31", "months", 1])).toBe("2026-02-28");
  expect(run("dateDifference", ["2026-02-28", "2026-03-02"])).toBe(2);
  expect(run("dateParts", ["2026-10-27"])).toMatchObject({
    year: 2026,
    month: 10,
    day: 27,
  });
  expect(run("percent", [2, 8])).toBe(25);
  expect(run("sin", [Math.PI / 2])).toBeCloseTo(1);
  expect(run("cos", [0])).toBe(1);
  expect(run("sqrt", [9])).toBe(3);
  expect(run("clamp", [7, 0, 5])).toBe(5);
  expect(run("payment", [1200, 0, 12])).toBe(100);
  expect(run("convertUnit", [1, "mile", "km"])).toBeCloseTo(1.609344);
  expect(run("movingAverage", [[2, 4, 6], 2])).toEqual([2, 3, 5]);
  expect(run("cumulative", [[2, 4, 6]])).toEqual([2, 6, 12]);
  expect(run("median", [[4, 2, 6, 8]])).toBe(5);
  expect(
    run("pivot", [
      [
        { row: "A", col: "B", value: 2 },
        { row: "A", col: "B", value: 3 },
      ],
      ["row"],
      ["col"],
      ["value"],
      "sum",
    ]),
  ).toEqual([{ key: "A", values: { B: 5 } }]);
  expect(
    validateField("2026-03-01", {}, "date", { max: "2026-02-28" }),
  ).toEqual({ code: "dateMaximum", limit: "2026-02-28" });
});

import { readFileSync } from "node:fs";
import { join } from "node:path";

import Ajv2020 from "ajv/dist/2020";
import { describe, expect, it } from "vitest";

import { WIRE_SAMPLES } from "@flame/runtime-contract/samples";

const CONTRACT = join(import.meta.dirname, "../../../../runtime/contract");
const SAMPLES = join(CONTRACT, "typescript", "samples");

function read(path: string): unknown {
  return JSON.parse(readFileSync(path, "utf8")) as unknown;
}

const ajv = new Ajv2020({ strict: false, allErrors: true });
const bundle = read(join(CONTRACT, "schema.json")) as { $defs: Record<string, unknown> };
ajv.addSchema(bundle, "schema.json");
const openrpc = read(join(CONTRACT, "openrpc.json")) as {
  methods: { name: string; params: { name: string }[] }[];
};
ajv.addSchema(openrpc, "openrpc.json");

function requestSchema(method: string, param?: string) {
  const index = openrpc.methods.findIndex((entry) => entry.name === method);
  const entry = openrpc.methods[index];
  if (entry === undefined) throw new Error(`OpenRPC defines no ${method}`);
  const fragment =
    param === undefined
      ? `methods/${index}/x-flame-requestFrame`
      : `methods/${index}/params/${entry.params.findIndex((parameter) => parameter.name === param)}/schema`;
  const validate = ajv.getSchema(`openrpc.json#/${fragment}`);
  if (!validate) throw new Error(`OpenRPC defines no ${method}.${param ?? "params"} schema`);
  return validate;
}

const pluginListIndex = openrpc.methods.findIndex((method) => method.name === "plugins.list");
const pluginThemeSchemas = [
  { reference: "schema.json#/$defs/PluginTheme", project: (theme: unknown) => theme },
  {
    reference: `openrpc.json#/methods/${pluginListIndex}/result/schema`,
    project: (theme: unknown) => ({
      data: [
        {
          id: "940ac827-b431-455b-af4b-e3a170bcfda0",
          source: "/sample",
          enabled: false,
          availability: [],
          grants: [],
          values: {},
          disabledServers: [],
          disabledSkills: [],
          selected: {
            digest: "1".repeat(64),
            name: "sample",
            requests: [],
            servers: [],
            inputs: [],
            themes: [theme],
            skills: [],
            diagnostics: [],
          },
        },
      ],
    }),
  },
];

describe.each(pluginThemeSchemas)("plugin theme schema $reference", ({ reference, project }) => {
  const validate = ajv.getSchema(reference);
  if (!validate) throw new Error(`Published contract defines no ${reference}`);
  const theme = { id: "sample", title: "Sample", scheme: "dark" };

  it("accepts every declared portable color", () => {
    expect(
      validate(
        project({
          ...theme,
          colors: {
            background: "#102030",
            foreground: "#ABCDEF",
            accent: "#102030",
            muted: "#102030",
            border: "#102030",
          },
        }),
      ),
    ).toBe(true);
  });

  it.each([
    { unexpected: "#102030" },
    { background: "url(https://example.invalid/image)" },
    { accent: "#abc" },
    { border: "" },
  ])("refuses invalid portable colors: %j", (colors) => {
    expect(validate(project({ ...theme, colors }))).toBe(false);
  });
});

describe("the published OpenRPC request schemas", () => {
  const target = {
    installationId: "940ac827-b431-455b-af4b-e3a170bcfda0",
    digest: "1".repeat(64),
  };

  it("compiles every whole request and by-name parameter", () => {
    for (const method of openrpc.methods) {
      expect(() => requestSchema(method.name)).not.toThrow();
      for (const param of method.params) {
        expect(() => requestSchema(method.name, param.name)).not.toThrow();
      }
    }
  });

  it("rejects unknown nested members in frames and by-name parameters", () => {
    const grants = [{ capability: "tools.invoke", targets: [], unexpected: true }];
    expect(requestSchema("plugins.approve")({ ...target, grants })).toBe(false);
    expect(requestSchema("plugins.approve", "grants")(grants)).toBe(false);
    const valueChanges = { token: { type: "set", value: "", unexpected: true } };
    expect(
      requestSchema("plugins.configure")({
        ...target,
        disabledServers: [],
        disabledSkills: [],
        valueChanges,
      }),
    ).toBe(false);
    expect(requestSchema("plugins.configure", "valueChanges")(valueChanges)).toBe(false);
  });

  it("applies request strictness to universal metadata", () => {
    const _meta = { clientInfo: { name: "editor", version: "1", unexpected: true } };
    expect(requestSchema("plugins.list")({ _meta })).toBe(false);
    expect(requestSchema("plugins.list", "_meta")(_meta)).toBe(false);
    expect(requestSchema("plugins.list")({ _meta: { unexpected: true } })).toBe(false);
  });

  it("accepts declared maps, metadata, and opaque tool arguments", () => {
    expect(
      requestSchema("plugins.configure")({
        ...target,
        disabledServers: [],
        disabledSkills: [],
        valueChanges: { token: { type: "set", value: "" }, obsolete: { type: "clear" } },
        _meta: { clientInfo: { name: "editor", version: "1" } },
      }),
    ).toBe(true);
    expect(
      requestSchema("tools.invoke")({
        name: "inspect",
        arguments: { type: "unrelated", nested: { unexpected: null } },
      }),
    ).toBe(true);
    expect(
      ajv.getSchema("schema.json#/$defs/PluginValueChange")?.({
        type: "clear",
        futureField: true,
      }),
    ).toBe(true);
  });
});

describe("the published JSON Schema bundle", () => {
  it("compiles", () => {
    expect(Object.keys(bundle.$defs).length).toBeGreaterThan(0);
    for (const name of Object.keys(bundle.$defs)) {
      expect(() => ajv.getSchema(`schema.json#/$defs/${name}`)).not.toThrow();
    }
  });

  it.each(WIRE_SAMPLES)("accepts $file as $shape", ({ file, shape }) => {
    const validate = ajv.getSchema(`schema.json#/$defs/${shape}`);
    if (!validate) throw new Error(`the bundle defines no ${shape}`);
    validate(read(join(SAMPLES, file)));
    expect(validate.errors ?? []).toEqual([]);
  });

  it("refuses a frame the runtime would not produce", () => {
    const session = ajv.getSchema("schema.json#/$defs/Session");
    expect(session?.({ title: "no id" })).toBe(false);

    const block = ajv.getSchema("schema.json#/$defs/ContentBlock");
    expect(block?.({ type: "text", text: "hello" })).toBe(true);
    expect(block?.({ type: "text", text: "hello", mime: "image/png" })).toBe(false);

    const cancel = ajv.getSchema("schema.json#/$defs/CancelRunResponse");
    const canceledRun = {
      id: "run_01",
      sessionId: "ses_01",
      status: "finished",
      outcome: { type: "canceled" },
      finishedAt: "2026-07-30T00:00:00Z",
      metrics: { steps: 0, activeDurationMillis: 0 },
      protocolProfile: { requiredFeatures: [], interruptTypes: [] },
      provider: "openai",
      model: "gpt-5",
      createdAt: "2026-07-30T00:00:00Z",
    };
    expect(cancel?.({ type: "root", run: canceledRun })).toBe(true);
    expect(cancel?.({ type: "root", run: canceledRun, rootRun: canceledRun })).toBe(false);
    expect(cancel?.({ type: "child", run: canceledRun })).toBe(false);

    const steer = ajv.getSchema("schema.json#/$defs/SteerRunRequest");
    expect(
      steer?.({
        runId: "run_01",
        expectedSegmentId: "seg_01",
        input: [
          { type: "text", text: "compare this" },
          { type: "image", mime: "image/png", data: "aW1hZ2U=" },
        ],
      }),
    ).toBe(true);
    expect(steer?.({ runId: "run_01", expectedSegmentId: "seg_01", input: [] })).toBe(false);
    expect(steer?.({ runId: "run_01", expectedSegmentId: "seg_01", message: "legacy" })).toBe(
      false,
    );

    const streamEvent = ajv.getSchema("schema.json#/$defs/StreamEvent");
    expect(streamEvent?.({ type: "vendor.preview" })).toBe(false);

    const clientCapabilities = ajv.getSchema("schema.json#/$defs/ClientCapabilities");
    expect(
      clientCapabilities?.({
        excludedEphemeralEvents: ["segment.progress", "item.delta"],
      }),
    ).toBe(true);
    expect(clientCapabilities?.({ excludedEphemeralEvents: ["item.completed"] })).toBe(false);

    const runtimeEvent = ajv.getSchema("schema.json#/$defs/RuntimeEvent");
    expect(runtimeEvent?.({ type: "files.changed", sequence: 1, paths: ["README.md"] })).toBe(true);
    expect(runtimeEvent?.({ type: "skills.changed", sequence: 0 })).toBe(false);
    expect(runtimeEvent?.({ type: "files.changed", sequence: 1, paths: [] })).toBe(false);
    expect(runtimeEvent?.({ type: "resync", sequence: 1 })).toBe(false);
    expect(runtimeEvent?.({ type: "resync", sequence: 1, topics: [] })).toBe(false);
    expect(runtimeEvent?.({ type: "sessions.changed", sequence: 1, sessionIds: [] })).toBe(false);

    const runtimeLimits = ajv.getSchema("schema.json#/$defs/RuntimeLimits");
    expect(
      runtimeLimits?.({
        idempotency: {
          namespace: "idp_fedcba9876543210fedcba9876543210",
          retentionSeconds: 86_400,
        },
        runReplay: {
          scope: "runtimeInstanceRootSegment",
          maxEvents: 2048,
          maxBytes: 16_777_216,
        },
        mcpAuthorizationAttempts: { retentionSeconds: 600 },
        runtimeSubscription: {
          maxTopics: 32,
          maxWatches: 32,
          maxPaths: 256,
          maxDirectoryEntries: 10000,
          maxFileBytes: 1048576,
        },
      }),
    ).toBe(true);
    expect(
      runtimeLimits?.({
        runtimeSubscription: {
          maxTopics: 32,
          maxWatches: 32,
          maxPaths: 256,
          maxDirectoryEntries: 10000,
          maxFileBytes: 1048576,
        },
      }),
    ).toBe(false);

    const pendingInterruptSet = ajv.getSchema("schema.json#/$defs/PendingInterruptSet");
    expect(
      pendingInterruptSet?.({
        rootRunId: "run_01",
        sessionId: "ses_01",
        interrupts: [],
        createdAt: "2026-07-30T00:00:00Z",
      }),
    ).toBe(false);

    const problem = ajv.getSchema("schema.json#/$defs/ProblemData");
    expect(
      problem?.({
        type: "capability_not_negotiated",
        requiredCapabilities: [],
      }),
    ).toBe(false);
    const repeatedRequirement = { type: "feature", name: "subagents" };
    expect(
      problem?.({
        type: "capability_not_negotiated",
        requiredCapabilities: [repeatedRequirement, repeatedRequirement],
      }),
    ).toBe(false);
    expect(problem?.({ type: "run_lost" })).toBe(true);
    expect(problem?.({ type: "mcp_dial_failed", detail: "connection failed" })).toBe(false);
    expect(problem?.({ type: "plugin:acme/model_timeout", retryAfterSeconds: 2 })).toBe(true);
    expect(problem?.({ type: "model_timeout" })).toBe(false);
    expect(problem?.({ type: "plugin:Acme/model_timeout" })).toBe(false);
    expect(
      problem?.({
        type: "run_lost",
        activeRun: { runId: "run_1", status: "running" },
      }),
    ).toBe(false);
    expect(problem?.({ type: "idempotency_in_progress", retryAfterSeconds: 0 })).toBe(false);
  });
});

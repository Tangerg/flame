import { describe, expect, it } from "vitest";
import { PermissionRequest } from "./elicitation";

function request(properties = {}, required: string[] = [], extra = {}) {
  return new PermissionRequest(
    JSON.stringify({
      mode: "form",
      message: "Answer the original request.",
      requestedSchema: { type: "object", properties, required },
      ...extra,
    }),
  );
}

describe("original computer permission contract", () => {
  it("offers only the native persistence scopes supplied by the engine", () => {
    const approval = request({}, [], {
      _meta: {
        codex_approval_kind: "mcp_tool_call",
        connector_id: "computer-use",
        tool_params: { app: "org.example.notes" },
        persist: ["session"],
        riskLevel: "high",
        warningSubtitle: "Review the application.",
      },
    });
    expect(approval.native).toEqual({
      app: "org.example.notes",
      persist: ["session"],
      riskLevel: "high",
      subtitle: "Review the application.",
    });
    expect(approval.reply('{"action":"accept","content":{}}')).toEqual({
      action: "accept",
      content: {},
    });
    expect(
      approval.reply(
        '{"action":"accept","content":{},"_meta":{"persist":"session"}}',
      ),
    ).toEqual({ action: "accept", content: {}, _meta: { persist: "session" } });
    expect(() =>
      approval.reply(
        '{"action":"accept","content":{},"_meta":{"persist":"always"}}',
      ),
    ).toThrow("not offered");
  });

  it("does not turn generic forms into native application grants", () => {
    const approval = request({}, [], { _meta: { persist: ["always"] } });
    expect(approval.native).toBeUndefined();
    expect(() =>
      approval.reply(
        '{"action":"accept","content":{},"_meta":{"persist":"always"}}',
      ),
    ).toThrow();
  });

  it("validates all supported flat field forms against the original schema", () => {
    const approval = request(
      {
        name: { type: "string", minLength: 2, maxLength: 5 },
        email: { type: "string", format: "email" },
        amount: { type: "number", minimum: 1, maximum: 2 },
        count: { type: "integer", minimum: 0 },
        enabled: { type: "boolean" },
        select: {
          type: "string",
          oneOf: [
            { const: "a", title: "A" },
            { const: "b", title: "B" },
          ],
        },
        many: {
          type: "array",
          items: { type: "string", enum: ["a", "b"] },
          minItems: 1,
          maxItems: 2,
        },
        titled: {
          type: "array",
          items: { anyOf: [{ const: "x", title: "X" }] },
        },
        date: { type: "string", format: "date" },
      },
      ["name", "amount", "enabled", "many"],
    );
    expect(
      approval.reply(
        '{"action":"accept","content":{"name":"Ada","email":"ada@example.org","amount":1.5,"count":2,"enabled":true,"select":"b","many":["a"],"titled":["x"],"date":"2026-10-28"}}',
      ).action,
    ).toBe("accept");
    const valid = { name: "Ada", amount: 1.5, enabled: true, many: ["a"] };
    for (const override of [
      { name: "a" },
      { amount: 3 },
      { count: 1.1 },
      { enabled: "yes" },
      { select: "outside" },
      { many: ["a", "a"] },
      { email: "bad" },
      { date: "2026-02-31" },
      { extra: "unknown" },
    ])
      expect(() =>
        approval.reply(
          JSON.stringify({
            action: "accept",
            content: { ...valid, ...override },
          }),
        ),
      ).toThrow();
  });

  it("refuses unsafe numeric values and duplicate JSON fields instead of rounding or choosing a winner", () => {
    const approval = request({ count: { type: "integer" } });
    expect(() =>
      approval.reply(
        '{"action":"accept","content":{"count":9007199254740993}}',
      ),
    ).toThrow();
    expect(() =>
      approval.reply('{"action":"decline","action":"accept","content":{}}'),
    ).toThrow();
  });

  it("does not silently submit defaults or permit missing required fields", () => {
    const approval = request({ accepted: { type: "boolean", default: true } }, [
      "accepted",
    ]);
    expect(() => approval.reply('{"action":"accept","content":{}}')).toThrow();
    expect(
      approval.reply('{"action":"accept","content":{"accepted":false}}')
        .content,
    ).toEqual({ accepted: false });
  });

  it.each(["decline", "cancel"])(
    "keeps %s distinct and refuses accompanying content or persistence",
    (action) => {
      const approval = request();
      expect(approval.reply(JSON.stringify({ action }))).toEqual({ action });
      expect(() =>
        approval.reply(JSON.stringify({ action, content: {} })),
      ).toThrow();
      expect(() =>
        approval.reply(
          JSON.stringify({ action, _meta: { persist: "always" } }),
        ),
      ).toThrow();
    },
  );

  it("uses explicit URL permission completion without inventing form content", () => {
    const approval = new PermissionRequest(
      JSON.stringify({
        mode: "url",
        message: "Sign in to the requested service.",
        url: "https://example.org/authorize",
        elicitationId: "original",
      }),
    );
    expect(approval.reply('{"action":"accept"}')).toEqual({ action: "accept" });
    expect(() => approval.reply('{"action":"accept","content":{}}')).toThrow();
  });

  it.each([
    "javascript:alert(1)",
    "file:///tmp/approval",
    "https://user:secret@example.org",
  ])("refuses unsafe permission links: %s", (url) => {
    expect(
      () =>
        new PermissionRequest(
          JSON.stringify({
            mode: "url",
            message: "Visit",
            url,
            elicitationId: "original",
          }),
        ),
    ).toThrow();
  });

  it("refuses unsupported request fields instead of silently dropping their meaning", () => {
    expect(() => request({}, [], { task: { ttl: 1000 } })).toThrow();
    expect(() =>
      request({ nested: { type: "object", properties: {} } }),
    ).toThrow();
    expect(() =>
      request({ name: { type: "string", pattern: "^sensitive$" } }),
    ).toThrow();
    expect(() => request({}, ["missing"])).toThrow();
  });

  it("keeps supported titled and legacy choice labels attached to their original values", () => {
    const approval = request({
      legacy: {
        type: "string",
        enum: ["first", "second"],
        enumNames: ["First label", "Second label"],
      },
      titled: {
        type: "string",
        oneOf: [
          { const: "first", title: "First label" },
          { const: "second", title: "Second label" },
        ],
      },
    });
    expect(approval.request.mode).not.toBe("url");
    if (approval.request.mode === "url") throw new Error("expected form");
    expect(approval.request.requestedSchema.properties.legacy).toEqual({
      type: "string",
      enum: ["first", "second"],
      enumNames: ["First label", "Second label"],
    });
    expect(
      approval.reply(
        '{"action":"accept","content":{"legacy":"second","titled":"first"}}',
      ).content,
    ).toEqual({ legacy: "second", titled: "first" });
  });

  it.each([
    { type: "string", enum: ["a"], oneOf: [{ const: "b", title: "B" }] },
    { type: "string", enum: ["a", "b"], enumNames: ["A"] },
    { type: "string", enumNames: ["orphan"] },
    { type: "string", enum: ["a", "a"] },
    { type: "string", enum: [] },
    { type: "string", enum: [1] },
    {
      type: "string",
      oneOf: [
        { const: "a", title: "A" },
        { const: "a", title: "Different label" },
      ],
    },
    {
      type: "string",
      oneOf: [{ const: "a", title: "A", pattern: "^secret$" }],
    },
    { type: "string", oneOf: "not choices" },
    {
      type: "array",
      items: {
        type: "string",
        enum: ["a"],
        anyOf: [{ const: "b", title: "B" }],
      },
    },
    { type: "array", items: { type: "string", enum: ["a"], minLength: 8 } },
    {
      type: "array",
      items: { anyOf: [{ const: "a", title: "A", maximum: 1 }] },
    },
    {
      type: "array",
      items: {
        anyOf: [
          { const: "a", title: "A" },
          { const: "a", title: "A again" },
        ],
      },
    },
  ])("refuses ambiguous or unsupported choice definitions: %j", (field) => {
    expect(() => request({ choice: field })).toThrow();
  });

  it.each([
    { type: "string", minLength: -1 },
    { type: "string", maxLength: 1.5 },
    { type: "string", minLength: 3, maxLength: 2 },
    { type: "string", enum: ["a"], minLength: "ignored by a permissive union" },
    { type: "string", enum: ["a", "b"], minLength: 2 },
    { type: "string", enum: ["invalid", "also invalid"], format: "email" },
    { type: "number", minimum: 4, maximum: 3 },
    { type: "integer", minimum: 4, maximum: 3 },
    { type: "integer", minimum: 1.1, maximum: 1.9 },
    { type: "array", items: { type: "string", enum: ["a"] }, minItems: -1 },
    { type: "array", items: { type: "string", enum: ["a"] }, maxItems: 1.5 },
    { type: "array", items: { type: "string", enum: ["a"] }, minItems: 2 },
    {
      type: "array",
      items: { type: "string", enum: ["a", "b"] },
      minItems: 2,
      maxItems: 1,
    },
  ])("refuses invalid or impossible permission constraints: %j", (field) => {
    expect(() => request({ value: field })).toThrow();
  });

  it.each([
    { type: "boolean", default: "yes" },
    { type: "integer", default: 1.5 },
    { type: "number", minimum: 1, default: 0 },
    { type: "string", minLength: 2, default: "a" },
    { type: "string", format: "email", default: "invalid" },
    { type: "string", enum: ["a"], default: "b" },
    {
      type: "array",
      items: { type: "string", enum: ["a"] },
      default: ["a", "a"],
    },
    {
      type: "array",
      items: { type: "string", enum: ["a"] },
      minItems: 1,
      default: [],
    },
  ])("refuses defaults outside the original field contract: %j", (field) => {
    expect(() => request({ value: field })).toThrow();
  });

  it("does not infer submitted optional values from valid defaults", () => {
    const approval = request({
      choice: { type: "string", enum: ["a", "b"], minLength: 1, default: "a" },
      count: { type: "integer", minimum: 1, maximum: 3, default: 2 },
    });
    expect(approval.reply('{"action":"accept","content":{}}').content).toEqual(
      {},
    );
    expect(
      approval.reply('{"action":"accept","content":{"choice":"b","count":3}}')
        .content,
    ).toEqual({ choice: "b", count: 3 });
  });

  it("refuses duplicate required declarations before presenting a form", () => {
    expect(() =>
      request({ name: { type: "string" } }, ["name", "name"]),
    ).toThrow("invalid permission form fields");
  });

  it("keeps the accepted request immutable and bounds its size", () => {
    const approval = request({ name: { type: "string" } });
    expect(Object.isFrozen(approval.request)).toBe(true);
    expect(() => request({}, [], { message: "x".repeat(65536) })).toThrow();
  });
});

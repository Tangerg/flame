import { describe, expect, it, vi } from "vitest";
import * as timeoutOwner from "./timeout";
import { type MCPServerSettings, userMCPServer } from "./model";
import { MCPServerEdit, RetainedValue, type MCPServerFields } from "./draft";
function editWith(
  fields: Partial<MCPServerFields>,
  server?: MCPServerSettings,
): MCPServerEdit {
  return Object.entries(fields).reduce<MCPServerEdit>(
    (edit, [key, value]) =>
      edit.with(key as keyof MCPServerFields, value as never),
    MCPServerEdit.of(server),
  );
}

describe("mcpServerDraft", () => {
  it("preserves exact process material when only description or timeout changes", () => {
    const server: MCPServerSettings = {
      id: userMCPServer("literal"),
      status: "disabled",
      type: "stdio",
      enabled: false,
      command: "/tools/mcp ",
      dir: "/work/project ",
      args: ["--label", " value ", "", "first\nsecond"],
      envMasked: { TOKEN: "********" },
      handshakeTimeout: { type: "unbounded" },
    };
    for (const fields of [{ description: "changed" }, { timeoutSec: "15" }]) {
      const edit = editWith(fields, server);
      expect(edit.isValid).toBe(true);
      expect(edit.environmentNeedsDisposition).toBe(false);
      expect(edit.toInput()).toMatchObject({
        command: server.command,
        dir: server.dir,
        args: server.args,
      });
      expect(edit.toInput().env).toBeUndefined();
      expect(edit.toUpdate()).not.toHaveProperty("connection");
    }
  });

  it("creates exact JSON arguments and environment values", () => {
    const args = ["", " space ", "first\nsecond", 'quote"\\tail'];
    const env = {
      EMPTY: "",
      SPACES: " \t ",
      TOKEN: " value ",
      " PADDED ": "first\nsecond",
    };
    const edit = editWith({
      name: "literal",
      command: " mcp ",
      dir: " /project ",
      args: JSON.stringify(args),
      environment: RetainedValue.preserved().edited(JSON.stringify(env)),
    });
    expect(edit.toInput()).toMatchObject({
      command: " mcp ",
      dir: " /project ",
      args,
      env,
    });
  });

  it.each([
    ["args", "one\ntwo"],
    ["args", "null"],
    ["args", '["ok", 2]'],
    ["environment", '{"KEY":"first","KEY":"second"}'],
    ["environment", '{"KEY":2}'],
    ["environment", "{}"],
    ["headers", "KEY=value"],
    ["headers", "[]"],
    ["headers", '{"X-Key":null}'],
  ] as const)(
    "rejects unrepresentable %s JSON %s without guessing",
    (field, source) => {
      const edit =
        field === "headers"
          ? editWith({
              name: "cloud",
              transport: "streamableHttp",
              url: "https://example.com",
              headers: RetainedValue.preserved().edited(source),
            })
          : editWith({
              name: "local",
              command: "node",
              ...(field === "args"
                ? { args: source }
                : { environment: RetainedValue.preserved().edited(source) }),
            });
      expect(edit.jsonIssue).toBe(field);
      expect(edit.isValid).toBe(false);
      expect(() => edit.toInput()).toThrow("mcp_form_json_invalid");
    },
  );

  it("keeps an unchanged edit empty and changes only the explicit secret disposition", () => {
    const server: MCPServerSettings = {
      id: userMCPServer("local"),
      type: "stdio",
      status: "disabled",
      enabled: false,
      command: "node",
      args: [""],
      envMasked: { TOKEN: "****" },
      handshakeTimeout: { type: "unbounded" },
    };
    const edit = MCPServerEdit.of(server);
    expect(edit.toUpdate()).toBeUndefined();
    expect(
      edit
        .with("environment", RetainedValue.preserved().cleared(true))
        .toUpdate(),
    ).toEqual({
      connection: {
        transport: "stdio",
        command: "node",
        args: [""],
        env: null,
        dir: undefined,
      },
    });
    expect(edit.with("args", '[" "]').environmentNeedsDisposition).toBe(true);
    expect(edit.with("args", "invalid").environmentNeedsDisposition).toBe(
      false,
    );
    expect(edit.with("args", "invalid").isValid).toBe(false);
  });

  it("builds stdio config input from the form draft", () => {
    const input = editWith({
      name: " git ",
      transport: "stdio",
      description: " repository tools ",
      command: " npx ",
      args: JSON.stringify(["-y", "@modelcontextprotocol/server-git"]),
      environment: RetainedValue.preserved().edited(
        '{"TOKEN":"a=b","EMPTY_KEY":""}',
      ),
      dir: " /repo ",
      url: "",
      authorization: RetainedValue.preserved().cleared(false),
      headers: RetainedValue.preserved().cleared(false),
      timeoutSec: "30",
    }).toInput();

    expect(input).toMatchObject({
      name: "git",
      transport: "stdio",
      enabled: true,
      description: "repository tools",
      command: " npx ",
      args: ["-y", "@modelcontextprotocol/server-git"],
      env: { TOKEN: "a=b", EMPTY_KEY: "" },
      dir: " /repo ",
      handshakeTimeout: { type: "bounded", seconds: 30 },
    });
  });

  it("carries an environment variable whose name is an inherited member", () => {
    const input = editWith({
      name: "srv",
      transport: "stdio",
      description: "",
      command: "run",
      args: "",
      environment: RetainedValue.preserved().edited(
        '{"__proto__":"polluted","constructor":"c","KEEP":"1"}',
      ),
      dir: "",
      url: "",
      authorization: RetainedValue.preserved().cleared(false),
      headers: RetainedValue.preserved().cleared(false),
      timeoutSec: "",
    }).toInput();

    expect(Object.keys(input.env ?? {}).sort()).toEqual([
      "KEEP",
      "__proto__",
      "constructor",
    ]);
    expect(Object.getPrototypeOf(input.env)).toBe(Object.prototype);
  });

  it("keeps blank http authorization omitted and parses extra headers", () => {
    const server: MCPServerSettings = {
      id: userMCPServer("cloud"),
      status: "disabled",
      type: "streamableHttp",
      enabled: false,
      handshakeTimeout: { type: "unbounded" },
      url: "https://example.com/mcp",
      authorizationMasked: "********",
    };
    const input = editWith(
      {
        name: " cloud ",
        transport: "streamableHttp",
        description: "",
        command: "",
        args: "",
        environment: RetainedValue.preserved().cleared(false),
        dir: "",
        url: " https://example.com/mcp ",
        authorization: RetainedValue.preserved().edited("   "),
        headers: RetainedValue.preserved().edited(
          '{"X-Trace":"abc=123","Bare":""}',
        ),
        timeoutSec: "",
      },
      server,
    ).toInput();

    expect(input).toMatchObject({
      name: "cloud",
      transport: "streamableHttp",
      enabled: false,
      url: "https://example.com/mcp",
      headers: { "X-Trace": "abc=123", Bare: "" },
    });
    expect(input.authorization).toBeUndefined();
    expect(input.handshakeTimeout).toEqual({ type: "unbounded" });
  });

  it("initializes editable text fields from an existing server", () => {
    const draft = MCPServerEdit.of({
      id: userMCPServer("fs"),
      status: "connected",
      type: "stdio",
      enabled: true,
      command: "node",
      args: ["server.js", "--root", "/repo"],
      envMasked: { A: "********", B: "********" },
      headersMasked: { "X-Env": "********" },
      handshakeTimeout: { type: "bounded", seconds: 15 },
    });

    expect(draft.fields).toMatchObject({
      name: "fs",
      transport: "stdio",
      command: "node",
      args: JSON.stringify(["server.js", "--root", "/repo"], null, 2),
      environment: { disposition: "preserve" },
      headers: { disposition: "preserve" },
      timeoutSec: "15",
      authorization: { disposition: "preserve" },
    });
  });

  it("requires an explicit credential decision when the HTTP origin changes", () => {
    const server: MCPServerSettings = {
      id: userMCPServer("cloud"),
      status: "disconnected",
      type: "streamableHttp",
      enabled: true,
      handshakeTimeout: { type: "unbounded" },
      url: "https://old.example/mcp",
      authorizationMasked: "********",
    };
    const draft = editWith({ url: "https://new.example/mcp" }, server);

    expect(draft.isValid).toBe(false);
    const cleared = draft.with(
      "authorization",
      RetainedValue.preserved().cleared(true),
    );
    expect(cleared.isValid).toBe(true);
    expect(
      draft.with(
        "authorization",
        RetainedValue.preserved().edited("Bearer replacement"),
      ).isValid,
    ).toBe(true);
    expect(cleared.toInput().authorization).toBe(null);
  });

  it("requires explicit dispositions for stored headers when the HTTP origin changes", () => {
    const server: MCPServerSettings = {
      id: userMCPServer("cloud"),
      status: "disconnected",
      type: "streamableHttp",
      enabled: true,
      handshakeTimeout: { type: "unbounded" },
      url: "https://old.example/mcp",
      headersMasked: { "X-API-Key": "********" },
    };
    const draft = editWith({ url: "https://new.example/mcp" }, server);

    expect(draft.isValid).toBe(false);
    const cleared = draft.with(
      "headers",
      RetainedValue.preserved().cleared(true),
    );
    expect(cleared.isValid).toBe(true);
    expect(
      draft.with(
        "headers",
        RetainedValue.preserved().edited('{"X-API-Key":"replacement"}'),
      ).isValid,
    ).toBe(true);
    expect(cleared.toInput().headers).toBe(null);
  });

  it("preserves stored environment only for an unchanged stdio process target", () => {
    const server: MCPServerSettings = {
      id: userMCPServer("fs"),
      status: "disconnected",
      type: "stdio",
      enabled: true,
      handshakeTimeout: { type: "unbounded" },
      command: "node",
      args: ["server.js"],
      dir: "/repo",
      envMasked: { API_KEY: "********" },
    };
    const unchanged = MCPServerEdit.of(server);

    expect(unchanged.isValid).toBe(true);
    expect(unchanged.toInput().env).toBeUndefined();

    const changed = unchanged.with("args", '["other.js"]');
    expect(changed.isValid).toBe(false);
    const cleared = changed.with(
      "environment",
      RetainedValue.preserved().cleared(true),
    );
    expect(cleared.isValid).toBe(true);
    expect(cleared.toInput().env).toBe(null);
    expect(
      changed
        .with(
          "environment",
          RetainedValue.preserved().edited('{"API_KEY":"replacement"}'),
        )
        .toInput().env,
    ).toEqual({ API_KEY: "replacement" });
  });

  it("validates the active transport's required field", () => {
    const stdio = (fields: Partial<MCPServerFields>) =>
      editWith({ name: "git", command: "npx", ...fields }).isValid;

    expect(stdio({})).toBe(true);
    expect(stdio({ timeoutSec: "0" })).toBe(false);
    expect(stdio({ timeoutSec: "1.5" })).toBe(false);
    expect(stdio({ timeoutSec: "15" })).toBe(true);
    expect(stdio({ command: "" })).toBe(false);
    expect(
      editWith({
        name: "cloud",
        transport: "streamableHttp",
        url: "https://example.com/mcp",
      }).isValid,
    ).toBe(true);
  });

  it("models retained values as one explicit disposition", () => {
    const replacement = RetainedValue.preserved().edited("  secret  ");
    expect(replacement.disposition).toBe("replace");
    expect(replacement.text).toBe("  secret  ");
    expect(replacement.submittedText()).toBe("  secret  ");

    const cleared = RetainedValue.preserved().cleared(true);
    expect(cleared.disposition).toBe("clear");
    expect(cleared.text).toBe("");
    expect(cleared.submittedText()).toBe(null);

    expect(RetainedValue.preserved().edited("   ").disposition).toBe(
      "preserve",
    );
    expect(RetainedValue.preserved().cleared(false).disposition).toBe(
      "preserve",
    );
    expect(RetainedValue.preserved().submittedText()).toBeUndefined();
  });
});

it("propagates an unexpected timeout construction failure instead of reporting invalid input", () => {
  const failure = new Error("Unexpected timeout construction failure");
  const factory = vi
    .spyOn(timeoutOwner, "boundedMCPHandshakeTimeout")
    .mockImplementationOnce(() => {
      throw failure;
    });
  try {
    expect(
      () =>
        editWith({ name: "local", command: "npx", timeoutSec: "15" }).isValid,
    ).toThrow(failure);
  } finally {
    factory.mockRestore();
  }
});

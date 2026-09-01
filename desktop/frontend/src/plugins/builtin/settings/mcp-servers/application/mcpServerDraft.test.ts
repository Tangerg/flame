import { describe, expect, it } from "vitest";
import type { MCPServerSettings } from "./mcpServerQueries";
import { MCPServerEdit, RetainedValue, type MCPServerFields } from "./mcpServerDraft";

/** Folds a partial form state onto an edit through the public transition, so these stay
 *  tests of the model rather than of an object literal shaped like it. */
function editWith(fields: Partial<MCPServerFields>, server?: MCPServerSettings): MCPServerEdit {
  return Object.entries(fields).reduce<MCPServerEdit>(
    (edit, [key, value]) => edit.with(key as keyof MCPServerFields, value as never),
    MCPServerEdit.of(server),
  );
}

describe("mcpServerDraft", () => {
  it("builds stdio config input from the form draft", () => {
    const input = editWith({
      name: " git ",
      transport: "stdio",
      description: " repository tools ",
      command: " npx ",
      args: " -y\n@modelcontextprotocol/server-git\n\n",
      environment: RetainedValue.preserved().edited("TOKEN=a=b\nEMPTY_KEY\n"),
      dir: " /repo ",
      url: "",
      authorization: RetainedValue.preserved().cleared(false),
      headers: RetainedValue.preserved().cleared(false),
      timeoutSec: "30",
      disabledTools: ["danger"],
      autoApproveTools: ["status"],
    }).toInput();

    expect(input).toMatchObject({
      name: "git",
      transport: "stdio",
      enabled: true,
      description: "repository tools",
      command: "npx",
      args: ["-y", "@modelcontextprotocol/server-git"],
      env: { TOKEN: "a=b", EMPTY_KEY: "" },
      dir: "/repo",
      handshakeTimeout: { type: "bounded", seconds: 30 },
      disabledTools: ["danger"],
      autoApproveTools: ["status"],
    });
  });

  // Assigning a typed name onto an object literal silently discards `__proto__` —
  // the setter takes the string and stores nothing — so the variable disappeared
  // between the form and the wire with no error anywhere.
  it("carries an environment variable whose name is an inherited member", () => {
    const input = editWith({
      name: "srv",
      transport: "stdio",
      description: "",
      command: "run",
      args: "",
      environment: RetainedValue.preserved().edited("__proto__=polluted\nconstructor=c\nKEEP=1"),
      dir: "",
      url: "",
      authorization: RetainedValue.preserved().cleared(false),
      headers: RetainedValue.preserved().cleared(false),
      timeoutSec: "",
      disabledTools: [],
      autoApproveTools: [],
    }).toInput();

    expect(Object.keys(input.env ?? {}).sort()).toEqual(["KEEP", "__proto__", "constructor"]);
    expect(Object.getPrototypeOf(input.env)).toBe(Object.prototype);
  });

  it("keeps blank http authorization omitted and parses extra headers", () => {
    const server: MCPServerSettings = {
      id: "cloud",
      name: "cloud",
      desc: "",
      tools: 0,
      status: "disabled",
      icon: "tool",
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
        headers: RetainedValue.preserved().edited("X-Trace=abc=123\nBare\n"),
        timeoutSec: "",
        disabledTools: [],
        autoApproveTools: [],
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
    expect(input.disabledTools).toBeUndefined();
    expect(input.autoApproveTools).toBeUndefined();
  });

  it("initializes editable text fields from an existing server", () => {
    const draft = MCPServerEdit.of({
      id: "fs",
      name: "fs",
      desc: "",
      tools: 0,
      status: "connected",
      icon: "folder",
      type: "stdio",
      enabled: true,
      command: "node",
      args: ["server.js", "--root", "/repo"],
      envMasked: { A: "********", B: "********" },
      headersMasked: { "X-Env": "********" },
      handshakeTimeout: { type: "bounded", seconds: 15 },
      disabledTools: ["delete"],
      autoApproveTools: ["read"],
    });

    expect(draft.fields).toMatchObject({
      name: "fs",
      transport: "stdio",
      command: "node",
      args: "server.js\n--root\n/repo",
      environment: { disposition: "preserve" },
      headers: { disposition: "preserve" },
      timeoutSec: "15",
      authorization: { disposition: "preserve" },
      disabledTools: ["delete"],
      autoApproveTools: ["read"],
    });
  });

  it("requires an explicit credential decision when the HTTP origin changes", () => {
    const server: MCPServerSettings = {
      id: "cloud",
      name: "cloud",
      desc: "",
      tools: 0,
      status: "disconnected",
      icon: "tool",
      type: "streamableHttp",
      enabled: true,
      handshakeTimeout: { type: "unbounded" },
      url: "https://old.example/mcp",
      authorizationMasked: "********",
    };
    const draft = editWith({ url: "https://new.example/mcp" }, server);

    expect(draft.isValid).toBe(false);
    const cleared = draft.with("authorization", RetainedValue.preserved().cleared(true));
    expect(cleared.isValid).toBe(true);
    expect(
      draft.with("authorization", RetainedValue.preserved().edited("Bearer replacement")).isValid,
    ).toBe(true);
    expect(cleared.toInput().authorization).toBe(null);
  });

  it("requires explicit dispositions for stored headers when the HTTP origin changes", () => {
    const server: MCPServerSettings = {
      id: "cloud",
      name: "cloud",
      desc: "",
      tools: 0,
      status: "disconnected",
      icon: "tool",
      type: "streamableHttp",
      enabled: true,
      handshakeTimeout: { type: "unbounded" },
      url: "https://old.example/mcp",
      headersMasked: { "X-API-Key": "********" },
    };
    const draft = editWith({ url: "https://new.example/mcp" }, server);

    expect(draft.isValid).toBe(false);
    const cleared = draft.with("headers", RetainedValue.preserved().cleared(true));
    expect(cleared.isValid).toBe(true);
    expect(
      draft.with("headers", RetainedValue.preserved().edited("X-API-Key=replacement")).isValid,
    ).toBe(true);
    expect(cleared.toInput().headers).toBe(null);
  });

  it("preserves stored environment only for an unchanged stdio process target", () => {
    const server: MCPServerSettings = {
      id: "fs",
      name: "fs",
      desc: "",
      tools: 0,
      status: "disconnected",
      icon: "folder",
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

    const changed = unchanged.with("args", "other.js");
    expect(changed.isValid).toBe(false);
    const cleared = changed.with("environment", RetainedValue.preserved().cleared(true));
    expect(cleared.isValid).toBe(true);
    expect(cleared.toInput().env).toBe(null);
    expect(
      changed.with("environment", RetainedValue.preserved().edited("API_KEY=replacement")).toInput()
        .env,
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

  // A credential the user has not touched must read as "leave it alone", never as "erase
  // it" — the two are one keystroke apart in a form and unrecoverable apart on the server.
  it("models retained values as one explicit disposition", () => {
    const replacement = RetainedValue.preserved().edited("  secret  ");
    expect(replacement.disposition).toBe("replace");
    expect(replacement.text).toBe("  secret  ");

    const cleared = RetainedValue.preserved().cleared(true);
    expect(cleared.disposition).toBe("clear");
    expect(cleared.text).toBe("");
    expect(cleared.submittedText()).toBe(null);

    expect(RetainedValue.preserved().edited("   ").disposition).toBe("preserve");
    expect(RetainedValue.preserved().cleared(false).disposition).toBe("preserve");
    expect(RetainedValue.preserved().submittedText()).toBeUndefined();
  });
});

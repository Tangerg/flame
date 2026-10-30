import { afterEach, expect, it, vi } from "vitest";
import { RpcError, RpcTransportError } from "@flame/runtime-client";
import type { ApprovalRule, MCPServer } from "@flame/runtime-client/wire";
import { MCPSettings, IMPORT } from "./settings";
import { MCPServerEdit } from "./draft";
import { serverKey, serverSettings } from "./model";
import { fixture, mutation, server } from "./testing";
const fixtures: ReturnType<typeof fixture>[] = [];
function create(initial?: MCPServer[]) {
  const f = fixture(initial);
  fixtures.push(f);
  const settings = new MCPSettings({
    runtimeIdentity: f.context.runtime,
    component: f.context.component,
    signal: f.source.signal,
  });
  return { ...f, settings };
}
afterEach(() => {
  for (const f of fixtures.splice(0)) f.dispose();
  vi.useRealTimers();
});
const unknown = () => new RpcTransportError("synthetic disconnect");
it("retains sensitive drafts across view closes and clears them at source retirement", () => {
  const f = create();
  const key = serverKey(server.id);
  const draft = MCPServerEdit.of(serverSettings(server)).with(
    "description",
    "private-draft",
  );
  f.settings.edit(f.context, key, draft);
  f.view.abort();
  const next = f.openView();
  expect(f.settings.getSnapshot().drafts.get(key)).toBe(draft);
  f.source.abort();
  expect(f.settings.getSnapshot().drafts.size).toBe(0);
  expect(() => f.settings.save(next.context, draft, server.id)).toThrow();
  expect(f.update).not.toHaveBeenCalled();
});
it("refuses contexts from another Runtime or installed component", () => {
  const f = create();
  const other = create();
  const draft = MCPServerEdit.of();
  expect(() => f.settings.edit(other.context, "add", draft)).toThrow(
    "another installed source",
  );
  expect(() =>
    f.settings.edit(
      { ...f.context, component: { ...f.context.component, digest: "other" } },
      "add",
      draft,
    ),
  ).toThrow("another installed source");
});
it("reconciles an original unknown write and blocks other writes to that exact server", async () => {
  const f = create();
  const key = serverKey(server.id);
  const retry = vi.fn().mockResolvedValue(server);
  const original = mutation(Promise.reject(unknown()), retry);
  f.update.mockReturnValueOnce(original);
  const draft = MCPServerEdit.of(serverSettings(server));
  f.settings.edit(f.context, key, draft);
  await expect(f.settings.save(f.context, draft, server.id)).resolves.toBe(
    false,
  );
  expect(f.settings.getSnapshot().commands.get(key)?.phase).toBe("unknown");
  await f.settings.setEnabled(f.context, server.id, false);
  expect(f.update).toHaveBeenCalledTimes(1);
  expect(f.tracked).toContain(original);
  f.view.abort();
  const next = f.openView();
  await expect(f.settings.recover(next.context, key)).resolves.toBe(true);
  expect(retry).toHaveBeenCalledWith({ signal: next.context.signal });
  expect(f.update).toHaveBeenCalledTimes(1);
  expect(f.settings.getSnapshot().drafts.has(key)).toBe(false);
});
it("retains the accepted server when an older read arrives and the refresh fails", async () => {
  const f = create();
  f.settings.readServers(f.context);
  await vi.waitFor(() =>
    expect(f.settings.getSnapshot().servers.data).toHaveLength(1),
  );
  const stale = Promise.withResolvers<{ data: MCPServer[] }>();
  f.list
    .mockReturnValueOnce(stale.promise)
    .mockRejectedValue(new Error("refresh unavailable"));
  f.settings.readServers(f.context);
  const saved = { ...server, description: "canonical response" };
  f.update.mockReturnValueOnce(mutation(Promise.resolve(saved)));
  await expect(
    f.settings.setEnabled(f.context, server.id, false),
  ).resolves.toBe(true);
  await vi.waitFor(() =>
    expect(f.settings.getSnapshot().servers.phase).toBe("failed"),
  );
  stale.resolve({ data: [server] });
  await Promise.resolve();
  expect(f.settings.getSnapshot().servers.data?.[0]?.description).toBe(
    "canonical response",
  );
  expect(f.settings.getSnapshot().feedback.size).toBe(0);
});
it("does not clear a later edit when an accepted save settles", async () => {
  const f = create();
  const key = serverKey(server.id);
  const draft = MCPServerEdit.of(serverSettings(server));
  const done = Promise.withResolvers<MCPServer>();
  f.update.mockReturnValueOnce(mutation(done.promise));
  f.settings.edit(f.context, key, draft);
  const saving = f.settings.save(f.context, draft, server.id);
  const later = draft.with("description", "later edit");
  f.settings.edit(f.context, key, later);
  done.resolve(server);
  await saving;
  expect(f.settings.getSnapshot().drafts.get(key)).toBe(later);
});
it("preserves a sequential import prefix and resumes the original uncertain command", async () => {
  const f = create([]);
  f.settings.setImportText(
    f.context,
    JSON.stringify({
      mcpServers: {
        first: { command: "first" },
        second: { command: "second" },
        third: { command: "third" },
      },
    }),
  );
  const second = {
    ...server,
    id: { origin: { type: "user" as const }, name: "second" },
  };
  const retry = vi.fn().mockResolvedValue(second);
  const first = f.create.getMockImplementation()!;
  f.create
    .mockImplementationOnce(first)
    .mockReturnValueOnce(mutation(Promise.reject(unknown()), retry));
  await expect(f.settings.importServers(f.context)).resolves.toBe(false);
  expect(f.create.mock.calls.map(([s]) => s.name)).toEqual(["first", "second"]);
  expect(f.settings.getSnapshot().importProgress).toEqual({
    completed: 1,
    total: 3,
  });
  await f.settings.importServers(f.context);
  expect(f.create).toHaveBeenCalledTimes(2);
  f.view.abort();
  const next = f.openView();
  await expect(f.settings.recover(next.context, IMPORT)).resolves.toBe(true);
  expect(f.create.mock.calls.map(([s]) => s.name)).toEqual([
    "first",
    "second",
    "third",
  ]);
  expect(retry).toHaveBeenCalledTimes(1);
  expect(f.settings.getSnapshot().importProgress).toEqual({
    completed: 3,
    total: 3,
  });
  expect(f.settings.getSnapshot().feedback.get(IMPORT)).toMatchObject({
    kind: "imported",
    count: 3,
  });
});
it("continues after a known import failure without creating the accepted prefix again", async () => {
  const f = create([]);
  f.settings.setImportText(
    f.context,
    JSON.stringify({
      mcpServers: {
        first: { command: "first" },
        second: { command: "second" },
      },
    }),
  );
  const implementation = f.create.getMockImplementation()!;
  f.create
    .mockImplementationOnce(implementation)
    .mockReturnValueOnce(mutation(Promise.reject(new Error("known refusal"))));
  await f.settings.importServers(f.context);
  expect(f.settings.getSnapshot().importProgress?.completed).toBe(1);
  await f.settings.importServers(f.context);
  expect(f.create.mock.calls.map(([s]) => s.name)).toEqual([
    "first",
    "second",
    "second",
  ]);
});
it("preflights every import before sending any server creation", () => {
  const f = create();
  f.settings.setImportText(
    f.context,
    JSON.stringify({
      mcpServers: { first: { command: "first" }, invalid: { type: "stdio" } },
    }),
  );
  expect(() => f.settings.importServers(f.context)).toThrow();
  expect(f.create).not.toHaveBeenCalled();
});
it("isolates user and installation server commands even when display names match", async () => {
  const f = create();
  const installed = {
    ...server.id,
    origin: { type: "installation" as const, installationId: "package-one" },
  };
  const done = Promise.withResolvers<void>();
  f.reconnect.mockReturnValueOnce(mutation(done.promise));
  const first = f.settings.reconnect(f.context, server.id);
  await f.settings.reconnect(f.context, installed);
  expect(f.reconnect).toHaveBeenCalledTimes(2);
  done.resolve();
  await first;
});
it("uses exact global allow rules and preserves the original sequential rule removal", async () => {
  const f = create();
  const base: ApprovalRule = {
    id: "one",
    decision: "allow",
    scope: "global",
    subject: { type: "all" },
    stale: false,
    modelName: "read",
    tool: { type: "mcp", server: server.id, name: "read" },
  };
  f.setRules([
    base,
    { ...base, id: "two" },
    { ...base, id: "project", scope: "project" },
    { ...base, id: "deny", decision: "deny" },
    {
      ...base,
      id: "other",
      tool: {
        type: "mcp",
        server: {
          name: "docs",
          origin: { type: "installation", installationId: "package-one" },
        },
        name: "read",
      },
    },
  ]);
  f.settings.readRules(f.context);
  await vi.waitFor(() =>
    expect(f.settings.getSnapshot().rules.phase).toBe("ready"),
  );
  const retry = vi.fn().mockResolvedValue(undefined);
  f.forgetRule.mockReturnValueOnce(mutation(Promise.reject(unknown()), retry));
  await f.settings.setAutoApprove(f.context, server.id, "read", false);
  expect(f.forgetRule.mock.calls.map(([id]) => id)).toEqual(["one"]);
  await f.settings.recover(f.context, serverKey(server.id));
  expect(f.forgetRule.mock.calls.map(([id]) => id)).toEqual(["one", "two"]);
  expect(retry).toHaveBeenCalledTimes(1);
});
it("submits auto-approve through the canonical approval operation", async () => {
  const f = create();
  f.settings.readRules(f.context);
  await vi.waitFor(() =>
    expect(f.settings.getSnapshot().rules.phase).toBe("ready"),
  );
  await f.settings.setAutoApprove(f.context, server.id, "read", true);
  expect(f.setRule).toHaveBeenCalledWith({
    tool: { type: "mcp", server: server.id, name: "read" },
    scope: "global",
    subject: { type: "all" },
    decision: "allow",
  });
});
it("keeps an authorization attempt identity and resumes polling after reopening", async () => {
  vi.useFakeTimers();
  const f = create();
  await f.settings.authorize(f.context, server.id);
  f.view.abort();
  const next = f.openView();
  f.settings.observe(next.context);
  await vi.advanceTimersByTimeAsync(500);
  expect(f.createAuthorization).toHaveBeenCalledTimes(1);
  expect(f.getAuthorization).toHaveBeenCalledWith(
    "attempt-one",
    expect.any(AbortSignal),
  );
  expect(
    f.settings.getSnapshot().authorizations.get(serverKey(server.id))?.status
      .type,
  ).toBe("succeeded");
});
it("does not publish a stale probe result into a later draft", async () => {
  const f = create();
  const key = serverKey(server.id);
  const draft = MCPServerEdit.of(serverSettings(server));
  f.settings.edit(f.context, key, draft);
  const result = Promise.withResolvers<{ outcome: "reachable" }>();
  f.test.mockReturnValueOnce(result.promise);
  const testing = f.settings.test(f.context, key, draft);
  f.settings.edit(
    f.context,
    key,
    draft.with("url", "https://changed.example.test"),
  );
  result.resolve({ outcome: "reachable" });
  await testing;
  expect(f.settings.getSnapshot().feedback.has(key)).toBe(false);
  expect(f.update).not.toHaveBeenCalled();
});

it("refuses a changed credential recipient before any server command", () => {
  const f = create();
  const draft = MCPServerEdit.of(serverSettings(server)).with(
    "url",
    "https://changed.example.test/mcp",
  );
  expect(() => f.settings.save(f.context, draft, server.id)).toThrow(
    "invalid MCP server draft",
  );
  expect(f.update).not.toHaveBeenCalled();
});

it("uses a canonical expired-attempt response to permit another authorization", async () => {
  vi.useFakeTimers();
  const f = create();
  f.getAuthorization.mockRejectedValueOnce(
    new RpcError({
      code: -32032,
      message: "mcp_authorization_attempt_not_found",
      data: { type: "mcp_authorization_attempt_not_found" },
    }),
  );
  await f.settings.authorize(f.context, server.id);
  await vi.advanceTimersByTimeAsync(500);
  expect(
    f.settings.getSnapshot().authorizations.has(serverKey(server.id)),
  ).toBe(false);
  expect(
    f.settings.getSnapshot().feedback.get(serverKey(server.id)),
  ).toMatchObject({ kind: "error" });
  await f.settings.authorize(f.context, server.id);
  expect(f.createAuthorization).toHaveBeenCalledTimes(2);
});

import { vi } from "vitest";
import type { ComponentContext } from "@flame/component-sdk";
import type { MutationPromise } from "@flame/runtime-client";
import type {
  ApprovalRule,
  MCPAuthorizationAttempt,
  MCPServer,
  MCPServerCandidate,
  MCPServerID,
  MCPTestResult,
  UpdateMCPServerRequest,
} from "@flame/runtime-client/wire";
import { sameMCPServer } from "./model";
export const server: MCPServer = {
  id: { origin: { type: "user" }, name: "docs" },
  connection: {
    type: "streamableHttp",
    url: "https://docs.example.test/mcp",
    authorizationMasked: "synthetic-mask",
  },
  status: { type: "connected", toolCount: 1 },
  handshakeTimeout: { type: "unbounded" },
};
export function mutation<T>(
  promise: Promise<T>,
  retry = vi.fn<MutationPromise<T>["retry"]>(),
): MutationPromise<T> {
  return Object.assign(promise, { idempotencyKey: "original-command", retry });
}
export function fixture(initial = [server]) {
  const source = new AbortController();
  const views: AbortController[] = [];
  let servers = initial;
  let rules: ApprovalRule[] = [];
  const list = vi.fn(async (_signal?: AbortSignal) => ({ data: servers }));
  const create = vi.fn(
    (input: MCPServerCandidate): MutationPromise<MCPServer> => {
      const connection = input.connection;
      const saved: MCPServer = {
        id: { origin: { type: "user" }, name: input.name },
        connection:
          connection.type === "stdio"
            ? {
                type: "stdio",
                command: connection.command,
                args: connection.args,
                dir: connection.dir,
              }
            : { type: "streamableHttp", url: connection.url },
        description: input.description,
        status: input.enabled ? { type: "disconnected" } : { type: "disabled" },
        handshakeTimeout: input.handshakeTimeout,
      };
      servers = [...servers, saved];
      return mutation(Promise.resolve(saved));
    },
  );
  const update = vi.fn(
    (input: UpdateMCPServerRequest): MutationPromise<MCPServer> => {
      const current = servers.find((s) => sameMCPServer(s.id, input.server))!;
      const saved = {
        ...current,
        status:
          input.enabled === false
            ? { type: "disabled" as const }
            : current.status,
      };
      servers = servers.map((s) =>
        sameMCPServer(s.id, input.server) ? saved : s,
      );
      return mutation(Promise.resolve(saved));
    },
  );
  const remove = vi.fn((_server: MCPServerID) => mutation(Promise.resolve()));
  const test = vi
    .fn<
      (
        _input: MCPServerCandidate,
        _signal?: AbortSignal,
      ) => Promise<MCPTestResult>
    >()
    .mockResolvedValue({ outcome: "reachable" });
  const reconnect = vi.fn((_server: MCPServerID) =>
    mutation(Promise.resolve()),
  );
  const tools = vi.fn(async (_server?: MCPServerID, _signal?: AbortSignal) => ({
    data: [
      {
        server: server.id,
        name: "read",
        description: "Read documentation",
        modelName: "read",
        nameConflicts: [],
      },
    ],
  }));
  const exposure = vi.fn(
    async (server: MCPServerID, _signal?: AbortSignal) => ({
      server,
      disabledTools: [] as string[],
    }),
  );
  const setExposure = vi.fn((_input: unknown) => mutation(Promise.resolve()));
  const listRules = vi.fn(
    async (_sessionId?: string, _signal?: AbortSignal) => ({ rules }),
  );
  const setRule = vi.fn((_input: unknown) => mutation(Promise.resolve()));
  const forgetRule = vi.fn((id: string) => {
    rules = rules.filter((r) => r.id !== id);
    return mutation(Promise.resolve());
  });
  const attempt: MCPAuthorizationAttempt = {
    id: "attempt-one",
    server: server.id,
    createdAt: "2026-10-30T00:00:00Z",
    status: { type: "pending" },
  };
  const createAuthorization = vi.fn(
    (_server: MCPServerID, _signal?: AbortSignal) =>
      mutation(Promise.resolve(attempt)),
  );
  const getAuthorization = vi.fn(
    async (
      _id: string,
      _signal?: AbortSignal,
    ): Promise<MCPAuthorizationAttempt> => ({
      ...attempt,
      status: { type: "succeeded" },
    }),
  );
  const subscribe = vi.fn(
    (_request: unknown, signal: AbortSignal) =>
      new Promise<never>((_resolve, reject) =>
        signal.addEventListener("abort", () => reject(signal.reason), {
          once: true,
        }),
      ),
  );
  const runtime = {
    mcp: {
      list,
      create,
      update,
      delete: remove,
      test,
      reconnect,
      listTools: tools,
      toolExposure: exposure,
      setToolExposure: setExposure,
      authorizationAttempts: {
        create: createAuthorization,
        get: getAuthorization,
      },
    },
    approval: { listRules, setRule, forgetRule },
    runtimeEvents: { subscribe },
  };
  const tracked: Promise<unknown>[] = [];
  function openView() {
    const view = new AbortController();
    views.push(view);
    const signal = AbortSignal.any([view.signal, source.signal]);
    const context = {
      runtime,
      signal,
      locale: "en",
      component: {
        installationId: "captured-installation",
        digest: "captured-release",
        componentId: "settings",
      },
      track: <P extends Promise<unknown>>(
        execute: (signal: AbortSignal) => P,
      ): P => {
        signal.throwIfAborted();
        const original = execute(signal);
        tracked.push(original);
        return original;
      },
    } as unknown as ComponentContext;
    return { view, context };
  }
  return {
    ...openView(),
    source,
    list,
    create,
    update,
    remove,
    test,
    reconnect,
    tools,
    exposure,
    setExposure,
    listRules,
    setRule,
    forgetRule,
    createAuthorization,
    getAuthorization,
    subscribe,
    tracked,
    openView,
    setRules(values: ApprovalRule[]) {
      rules = values;
    },
    dispose() {
      source.abort();
      for (const view of views) view.abort();
    },
  };
}

import {
  mutationSettlementIsUnknown,
  isErrorType,
  type ComponentContext,
  type ComponentSource,
} from "@flame/component-sdk";
import type { MutationPromise } from "@flame/runtime-client";
import type {
  ApprovalRule,
  MCPAuthorizationAttempt,
  MCPServer,
  MCPServerID,
  MCPTool,
  MCPTestResult,
} from "@flame/runtime-client/wire";
import {
  candidate,
  sameMCPServer,
  serverKey,
  serverSettings,
  updateRequest,
  userMCPServer,
  type MCPServerSettings,
} from "./model";
import { MCPServerEdit } from "./draft";
import { parseMcpImport, type McpImportResult } from "./import";

export type Read<T> =
  | { phase: "loading"; data?: T }
  | { phase: "ready"; data: T }
  | { phase: "failed"; data?: T; error: unknown };
export type Command =
  { phase: "running" } | { phase: "unknown"; error: unknown };
export type Feedback =
  | { kind: "error"; error: unknown }
  | { kind: "tested"; outcome: MCPTestResult["outcome"] }
  | { kind: "imported"; count: number; renamed: McpImportResult["renamed"] };
interface Snapshot {
  servers: Read<MCPServerSettings[]>;
  tools: ReadonlyMap<string, Read<MCPTool[]>>;
  exposure: ReadonlyMap<string, Read<string[]>>;
  rules: Read<ApprovalRule[]>;
  drafts: ReadonlyMap<string, MCPServerEdit>;
  commands: ReadonlyMap<string, Command>;
  feedback: ReadonlyMap<string, Feedback>;
  probing: ReadonlySet<string>;
  authorizations: ReadonlyMap<string, MCPAuthorizationAttempt>;
  importText: string;
  importProgress?: { completed: number; total: number };
  observationFailure?: { error: unknown };
}
interface Step {
  execute(context: ComponentContext): MutationPromise<unknown>;
  accept(value: unknown): void;
}
interface Batch {
  resources: string[];
  steps: Step[];
  next: number;
  original?: MutationPromise<unknown>;
}
const ADD = "add";
export const IMPORT = "import";
export const editorKey = (server?: MCPServerID) =>
  server ? serverKey(server) : ADD;
const initial = (): Snapshot => ({
  servers: { phase: "loading" },
  tools: new Map(),
  exposure: new Map(),
  rules: { phase: "loading" },
  drafts: new Map(),
  commands: new Map(),
  feedback: new Map(),
  probing: new Set(),
  authorizations: new Map(),
  importText: "",
});

export class MCPSettings {
  readonly #source: ComponentSource;
  readonly #listeners = new Set<() => void>();
  readonly #reads = new Map<string, AbortController>();
  readonly #batches = new Map<string, Batch>();
  readonly #polling = new Map<string, AbortSignal>();
  #snapshot = initial();
  #import?: McpImportResult;
  constructor(source: ComponentSource) {
    source.signal.throwIfAborted();
    this.#source = source;
    source.signal.addEventListener(
      "abort",
      () => {
        for (const read of this.#reads.values()) read.abort();
        this.#reads.clear();
        this.#batches.clear();
        this.#polling.clear();
        this.#import = undefined;
        this.#snapshot = initial();
        this.#publish();
      },
      { once: true },
    );
  }
  getSnapshot = () => this.#snapshot;
  subscribe = (listener: () => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };
  observe(context: ComponentContext) {
    this.#assert(context);
    const controller = new AbortController();
    const signal = AbortSignal.any([
      controller.signal,
      context.signal,
      this.#source.signal,
    ]);
    this.#snapshot = { ...this.#snapshot, observationFailure: undefined };
    this.refresh(context, signal);
    for (const attempt of this.#snapshot.authorizations.values())
      if (attempt.status.type === "pending")
        this.#poll(context, attempt, signal);
    void context.track(async () => {
      try {
        const subscription = await context.runtime.runtimeEvents.subscribe(
          { topics: ["mcp.changed", "plugins.changed", "approvals.changed"] },
          signal,
        );
        if (signal.aborted) return;
        this.refresh(context, signal);
        for await (const _event of subscription.events) {
          if (signal.aborted) break;
          this.refresh(context, signal);
        }
      } catch (error) {
        if (!signal.aborted) {
          this.#snapshot = { ...this.#snapshot, observationFailure: { error } };
          this.#publish();
        }
      }
    });
    return () => controller.abort();
  }
  refresh(context: ComponentContext, signal = context.signal) {
    this.readServers(context, signal);
    this.readRules(context, signal);
    const servers = this.#snapshot.servers.data;
    for (const server of servers ?? [])
      if (this.#snapshot.tools.has(serverKey(server.id)))
        this.readTools(context, server.id, signal);
  }
  readServers(context: ComponentContext, signal = context.signal) {
    this.#read(
      context,
      "servers",
      signal,
      this.#snapshot.servers,
      async (signal) =>
        (await context.runtime.mcp.list(signal)).data.map(serverSettings),
      (servers) => {
        this.#snapshot = { ...this.#snapshot, servers };
      },
    );
  }
  readRules(context: ComponentContext, signal = context.signal) {
    this.#read(
      context,
      "rules",
      signal,
      this.#snapshot.rules,
      async (signal) =>
        (await context.runtime.approval.listRules(undefined, signal)).rules,
      (rules) => {
        this.#snapshot = { ...this.#snapshot, rules };
      },
    );
  }
  readTools(
    context: ComponentContext,
    server: MCPServerID,
    signal = context.signal,
  ) {
    const key = serverKey(server);
    this.#read(
      context,
      `tools:${key}`,
      signal,
      this.#snapshot.tools.get(key),
      async (signal) =>
        (await context.runtime.mcp.listTools(server, signal)).data,
      (read) => {
        this.#snapshot = {
          ...this.#snapshot,
          tools: new Map(this.#snapshot.tools).set(key, read),
        };
      },
    );
    this.#read(
      context,
      `exposure:${key}`,
      signal,
      this.#snapshot.exposure.get(key),
      async (signal) =>
        (await context.runtime.mcp.toolExposure(server, signal)).disabledTools,
      (read) => {
        this.#snapshot = {
          ...this.#snapshot,
          exposure: new Map(this.#snapshot.exposure).set(key, read),
        };
      },
    );
  }
  edit(context: ComponentContext, key: string, draft: MCPServerEdit) {
    this.#assert(context);
    this.#snapshot = {
      ...this.#snapshot,
      drafts: new Map(this.#snapshot.drafts).set(key, draft),
    };
    this.#feedback(key);
    this.#publish();
  }
  save(context: ComponentContext, draft: MCPServerEdit, server?: MCPServerID) {
    this.#assert(context);
    if (!draft.isValid) throw new Error("invalid MCP server draft");
    const input = draft.toInput();
    const key = editorKey(server);
    const target = server ?? userMCPServer(input.name);
    return this.#start(
      context,
      key,
      [serverKey(target)],
      [
        {
          execute: (context) =>
            server
              ? context.runtime.mcp.update(updateRequest(server, input))
              : context.runtime.mcp.create(candidate(input)),
          accept: (value) => {
            this.#acceptServer(value as MCPServer);
            if (
              !this.#snapshot.drafts.has(key) ||
              this.#snapshot.drafts.get(key) === draft
            ) {
              const drafts = new Map(this.#snapshot.drafts);
              drafts.delete(key);
              this.#snapshot = { ...this.#snapshot, drafts };
            }
          },
        },
      ],
    );
  }
  remove(context: ComponentContext, server: MCPServerID) {
    const key = serverKey(server);
    return this.#start(
      context,
      key,
      [key],
      [
        {
          execute: (context) => context.runtime.mcp.delete(server),
          accept: () => {
            this.#cancelRead("servers");
            const servers = this.#snapshot.servers.data;
            if (servers)
              this.#snapshot = {
                ...this.#snapshot,
                servers: {
                  phase: "ready",
                  data: servers.filter((s) => !sameMCPServer(s.id, server)),
                },
              };
            const drafts = new Map(this.#snapshot.drafts);
            drafts.delete(key);
            this.#snapshot = { ...this.#snapshot, drafts };
          },
        },
      ],
    );
  }
  setEnabled(context: ComponentContext, server: MCPServerID, enabled: boolean) {
    const key = serverKey(server);
    return this.#start(
      context,
      key,
      [key],
      [
        {
          execute: (context) => context.runtime.mcp.update({ server, enabled }),
          accept: (value) => this.#acceptServer(value as MCPServer),
        },
      ],
    );
  }
  reconnect(context: ComponentContext, server: MCPServerID) {
    const key = serverKey(server);
    return this.#start(
      context,
      key,
      [key],
      [
        {
          execute: (context) => context.runtime.mcp.reconnect(server),
          accept: () => {},
        },
      ],
    );
  }
  setExposure(
    context: ComponentContext,
    server: MCPServerID,
    name: string,
    disabled: boolean,
  ) {
    const key = serverKey(server);
    return this.#start(
      context,
      key,
      [key],
      [
        {
          execute: (context) =>
            context.runtime.mcp.setToolExposure({ server, name, disabled }),
          accept: () => {
            this.#cancelRead(`exposure:${key}`);
            const data = this.#snapshot.exposure.get(key)?.data;
            if (data)
              this.#snapshot = {
                ...this.#snapshot,
                exposure: new Map(this.#snapshot.exposure).set(key, {
                  phase: "ready",
                  data: disabled
                    ? [...new Set([...data, name])]
                    : data.filter((n) => n !== name),
                }),
              };
          },
        },
      ],
    );
  }
  setAutoApprove(
    context: ComponentContext,
    server: MCPServerID,
    name: string,
    on: boolean,
  ) {
    const key = serverKey(server);
    const rules = this.#snapshot.rules;
    if (rules.phase !== "ready") return Promise.resolve(false);
    const steps: Step[] = on
      ? [
          {
            execute: (context) =>
              context.runtime.approval.setRule({
                tool: { type: "mcp", server, name },
                scope: "global",
                subject: { type: "all" },
                decision: "allow",
              }),
            accept: () => {},
          },
        ]
      : this.allowRules(server, name).map((rule) => ({
          execute: (context) => context.runtime.approval.forgetRule(rule.id),
          accept: () => {
            this.#cancelRead("rules");
            const data = this.#snapshot.rules.data;
            if (data)
              this.#snapshot = {
                ...this.#snapshot,
                rules: {
                  phase: "ready",
                  data: data.filter((r) => r.id !== rule.id),
                },
              };
          },
        }));
    return this.#start(context, key, [key], steps);
  }
  allowRules(server: MCPServerID, name: string) {
    return (
      this.#snapshot.rules.data?.filter(
        (rule) =>
          rule.tool.type === "mcp" &&
          sameMCPServer(rule.tool.server, server) &&
          rule.tool.name === name &&
          rule.scope === "global" &&
          rule.subject.type === "all" &&
          rule.decision === "allow",
      ) ?? []
    );
  }
  authorize(context: ComponentContext, server: MCPServerID) {
    const key = serverKey(server);
    return this.#start(
      context,
      key,
      [key],
      [
        {
          execute: (context) =>
            context.runtime.mcp.authorizationAttempts.create(
              server,
              context.signal,
            ),
          accept: (value) => {
            const attempt = value as MCPAuthorizationAttempt;
            this.#snapshot = {
              ...this.#snapshot,
              authorizations: new Map(this.#snapshot.authorizations).set(
                key,
                attempt,
              ),
            };
          },
        },
      ],
    );
  }
  async test(context: ComponentContext, key: string, draft: MCPServerEdit) {
    this.#assert(context);
    if (this.busy(key) || this.#snapshot.probing.has(key)) return;
    this.#snapshot = {
      ...this.#snapshot,
      probing: new Set(this.#snapshot.probing).add(key),
    };
    this.#feedback(key);
    this.#publish();
    const signal = AbortSignal.any([context.signal, this.#source.signal]);
    try {
      const result = await context.track(() =>
        context.runtime.mcp.test(candidate(draft.toInput()), signal),
      );
      if (
        !signal.aborted &&
        (!this.#snapshot.drafts.has(key) ||
          this.#snapshot.drafts.get(key) === draft)
      )
        this.#feedback(key, { kind: "tested", outcome: result.outcome });
    } catch (error) {
      if (
        !signal.aborted &&
        (!this.#snapshot.drafts.has(key) ||
          this.#snapshot.drafts.get(key) === draft)
      )
        this.#feedback(key, { kind: "error", error });
    } finally {
      if (!this.#source.signal.aborted) {
        const probing = new Set(this.#snapshot.probing);
        probing.delete(key);
        this.#snapshot = { ...this.#snapshot, probing };
        this.#publish();
      }
    }
  }
  setImportText(context: ComponentContext, text: string) {
    this.#assert(context);
    if (this.#batches.has(IMPORT)) return;
    this.#import = undefined;
    this.#snapshot = {
      ...this.#snapshot,
      importText: text,
      importProgress: undefined,
    };
    this.#feedback(IMPORT);
    this.#publish();
  }
  importServers(context: ComponentContext) {
    this.#assert(context);
    if (this.#batches.has(IMPORT)) return Promise.resolve(false);
    this.#import ??= parseMcpImport(this.#snapshot.importText);
    const parsed = this.#import;
    const completed = this.#snapshot.importProgress?.completed ?? 0;
    this.#snapshot = {
      ...this.#snapshot,
      importProgress: { completed, total: parsed.servers.length },
    };
    return this.#start(
      context,
      IMPORT,
      parsed.servers
        .slice(completed)
        .map((s) => serverKey(userMCPServer(s.name))),
      parsed.servers.slice(completed).map((input) => ({
        execute: (context) => context.runtime.mcp.create(candidate(input)),
        accept: (value) => {
          this.#acceptServer(value as MCPServer);
          this.#snapshot = {
            ...this.#snapshot,
            importProgress: {
              completed: (this.#snapshot.importProgress?.completed ?? 0) + 1,
              total: parsed.servers.length,
            },
          };
        },
      })),
    );
  }
  busy(resource: string): boolean {
    return [...this.#batches].some(
      ([key, batch]) => key === resource || batch.resources.includes(resource),
    );
  }
  recover(context: ComponentContext, key: string) {
    this.#assert(context);
    const batch = this.#batches.get(key);
    if (!batch || this.#snapshot.commands.get(key)?.phase !== "unknown")
      return Promise.resolve(false);
    return this.#run(context, key, batch);
  }
  #start(
    context: ComponentContext,
    key: string,
    resources: string[],
    steps: Step[],
  ) {
    this.#assert(context);
    if (
      this.busy(key) ||
      resources.some((resource) => this.busy(resource)) ||
      this.#snapshot.probing.has(key)
    )
      return Promise.resolve(false);
    this.#feedback(key);
    const batch: Batch = { resources, steps, next: 0 };
    this.#batches.set(key, batch);
    return this.#run(context, key, batch);
  }
  async #run(context: ComponentContext, key: string, batch: Batch) {
    this.#command(key, { phase: "running" });
    try {
      while (batch.next < batch.steps.length) {
        context.signal.throwIfAborted();
        const step = batch.steps[batch.next]!;
        const result = batch.original
          ? await context.track(() =>
              batch.original!.retry({ signal: context.signal }),
            )
          : await (batch.original = context.track(() => step.execute(context)));
        if (this.#source.signal.aborted || this.#batches.get(key) !== batch)
          return false;
        step.accept(result);
        batch.next++;
        batch.original = undefined;
        this.#publish();
      }
      this.#batches.delete(key);
      this.#command(key);
      if (key === IMPORT && this.#import) {
        this.#feedback(key, {
          kind: "imported",
          count: this.#import.servers.length,
          renamed: this.#import.renamed,
        });
        this.#publish();
      }
      if (!context.signal.aborted) {
        this.refresh(context);
        const attempt = this.#snapshot.authorizations.get(key);
        if (attempt?.status.type === "pending")
          this.#poll(context, attempt, context.signal);
      }
      return true;
    } catch (error) {
      if (this.#source.signal.aborted || this.#batches.get(key) !== batch)
        return false;
      if (mutationSettlementIsUnknown(error) || context.signal.aborted)
        this.#command(key, { phase: "unknown", error });
      else {
        this.#batches.delete(key);
        this.#feedback(key, { kind: "error", error });
        this.#command(key);
      }
      return false;
    }
  }
  retryAuthorization(context: ComponentContext, server: MCPServerID) {
    this.#assert(context);
    const attempt = this.#snapshot.authorizations.get(serverKey(server));
    if (attempt?.status.type === "pending") {
      this.#feedback(serverKey(server));
      this.#publish();
      this.#poll(context, attempt, context.signal);
    }
  }
  #poll(
    context: ComponentContext,
    initialAttempt: MCPAuthorizationAttempt,
    parent: AbortSignal,
  ) {
    const previous = this.#polling.get(initialAttempt.id);
    if (previous && !previous.aborted) return;
    const signal = AbortSignal.any([
      parent,
      context.signal,
      this.#source.signal,
    ]);
    this.#polling.set(initialAttempt.id, signal);
    void context.track(async () => {
      let attempt = initialAttempt;
      const key = serverKey(attempt.server);
      try {
        while (attempt.status.type === "pending") {
          await pollDelay(signal);
          attempt = await context.runtime.mcp.authorizationAttempts.get(
            attempt.id,
            signal,
          );
          if (signal.aborted) return;
          this.#snapshot = {
            ...this.#snapshot,
            authorizations: new Map(this.#snapshot.authorizations).set(
              key,
              attempt,
            ),
          };
          this.#publish();
        }
        this.refresh(context, signal);
      } catch (error) {
        if (!signal.aborted) {
          if (
            isErrorType(error, "mcp_authorization_attempt_not_found") &&
            this.#snapshot.authorizations.get(key)?.id === initialAttempt.id
          ) {
            const authorizations = new Map(this.#snapshot.authorizations);
            authorizations.delete(key);
            this.#snapshot = { ...this.#snapshot, authorizations };
            this.readServers(context, signal);
          }
          this.#feedback(key, { kind: "error", error });
          this.#publish();
        }
      } finally {
        if (this.#polling.get(initialAttempt.id) === signal)
          this.#polling.delete(initialAttempt.id);
      }
    });
  }
  #read<T>(
    context: ComponentContext,
    key: string,
    parent: AbortSignal,
    previous: Read<T> | undefined,
    execute: (signal: AbortSignal) => Promise<T>,
    accept: (read: Read<T>) => void,
  ) {
    if (this.#source.signal.aborted || context.signal.aborted || parent.aborted)
      return;
    this.#assert(context);
    this.#cancelRead(key);
    const controller = new AbortController();
    this.#reads.set(key, controller);
    const signal = AbortSignal.any([
      parent,
      context.signal,
      this.#source.signal,
      controller.signal,
    ]);
    accept({ phase: "loading", data: previous?.data });
    this.#publish();
    void context.track(async () => {
      try {
        const data = await execute(signal);
        if (!signal.aborted) accept({ phase: "ready", data });
      } catch (error) {
        if (!signal.aborted)
          accept({ phase: "failed", data: previous?.data, error });
      } finally {
        if (this.#reads.get(key) === controller) this.#reads.delete(key);
        if (!signal.aborted) this.#publish();
      }
    });
  }
  #acceptServer(raw: MCPServer) {
    this.#cancelRead("servers");
    const saved = serverSettings(raw);
    const current = this.#snapshot.servers.data;
    if (current)
      this.#snapshot = {
        ...this.#snapshot,
        servers: {
          phase: "ready",
          data: current.some((s) => sameMCPServer(s.id, saved.id))
            ? current.map((s) => (sameMCPServer(s.id, saved.id) ? saved : s))
            : [...current, saved],
        },
      };
  }
  #assert(context: ComponentContext) {
    this.#source.signal.throwIfAborted();
    context.signal.throwIfAborted();
    const component = context.component;
    if (
      context.runtime !== this.#source.runtimeIdentity ||
      component.installationId !== this.#source.component.installationId ||
      component.digest !== this.#source.component.digest ||
      component.componentId !== this.#source.component.componentId
    )
      throw new Error("mcp settings belong to another installed source");
  }
  #cancelRead(key: string) {
    this.#reads.get(key)?.abort();
    this.#reads.delete(key);
  }
  #command(key: string, value?: Command) {
    const commands = new Map(this.#snapshot.commands);
    if (value) commands.set(key, value);
    else commands.delete(key);
    this.#snapshot = { ...this.#snapshot, commands };
    this.#publish();
  }
  #feedback(key: string, value?: Feedback) {
    const feedback = new Map(this.#snapshot.feedback);
    if (value) feedback.set(key, value);
    else feedback.delete(key);
    this.#snapshot = { ...this.#snapshot, feedback };
  }
  #publish() {
    for (const listener of this.#listeners) listener();
  }
}
function pollDelay(signal: AbortSignal): Promise<void> {
  signal.throwIfAborted();
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      signal.removeEventListener("abort", abort);
      resolve();
    }, 500);
    function abort() {
      clearTimeout(timer);
      reject(signal.reason);
    }
    signal.addEventListener("abort", abort, { once: true });
  });
}

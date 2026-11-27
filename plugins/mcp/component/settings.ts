import {
  mutationSettlementIsUnknown,
  isErrorType,
  type ComponentSource,
  watchRuntimeTopics,
} from "@flame/component-sdk";
import type {
  ApprovalRule,
  MCPAuthorizationAttempt,
  MCPAuthorizationAttemptRef,
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
import { parseMcpImport } from "./import";
import {
  CommandBatch,
  type CommandStep,
  type SettingsContext,
} from "./command";

export type Read<T> =
  | { phase: "loading"; data?: T }
  | { phase: "ready"; data: T }
  | { phase: "failed"; data?: T; error: unknown };
type Command =
  | { readonly phase: "running" }
  | { readonly phase: "unknown"; readonly error: unknown };
type ActiveCommand = Command & { readonly batch: CommandBatch };
export type Feedback =
  | { kind: "error"; error: unknown }
  | { kind: "tested"; outcome: MCPTestResult["outcome"] };
interface AuthorizationObservation {
  server: MCPServerID;
  reference: MCPAuthorizationAttemptRef;
  attempt?: MCPAuthorizationAttempt;
}

export function shouldObserveAuthorization(
  observation: AuthorizationObservation,
): boolean {
  return (
    observation.attempt === undefined ||
    observation.attempt.status.type === "pending"
  );
}

interface Snapshot {
  servers: Read<MCPServerSettings[]>;
  tools: ReadonlyMap<string, Read<MCPTool[]>>;
  exposure: ReadonlyMap<string, Read<string[]>>;
  rules: Read<ApprovalRule[]>;
  drafts: ReadonlyMap<string, MCPServerEdit>;
  commands: ReadonlyMap<string, Command>;
  feedback: ReadonlyMap<string, Feedback>;
  probing: ReadonlySet<string>;
  authorizations: ReadonlyMap<string, AuthorizationObservation>;
  importText: string;
  importProgress?: CommandBatch["progress"];
  observationFailure?: { error: unknown };
}
interface SettingsState extends Snapshot {
  commands: ReadonlyMap<string, ActiveCommand>;
}
const ADD = "add";
export const IMPORT = "import";
export const editorKey = (server?: MCPServerID) =>
  server ? serverKey(server) : ADD;
export const toolPolicyKey = (server: MCPServerID) =>
  `tools:${serverKey(server)}`;
const initial = (): SettingsState => ({
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
  readonly #polling = new Map<MCPAuthorizationAttemptRef, AbortSignal>();
  #snapshot = initial();
  #import?: CommandBatch;
  constructor(source: ComponentSource) {
    source.signal.throwIfAborted();
    this.#source = source;
    source.signal.addEventListener(
      "abort",
      () => {
        for (const read of this.#reads.values()) read.abort();
        this.#reads.clear();
        this.#polling.clear();
        this.#import = undefined;
        this.#snapshot = initial();
        this.#publish();
      },
      { once: true },
    );
  }
  getSnapshot = (): Snapshot => this.#snapshot;
  subscribe = (listener: () => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };
  observe(context: SettingsContext) {
    this.#assert(context);
    const controller = new AbortController();
    const signal = AbortSignal.any([
      controller.signal,
      context.signal,
      this.#source.signal,
    ]);
    this.#snapshot = { ...this.#snapshot, observationFailure: undefined };
    this.refresh(context, signal);
    for (const authorization of this.#snapshot.authorizations.values())
      if (shouldObserveAuthorization(authorization))
        this.#poll(context, authorization, signal);
    void context.track(async () => {
      try {
        await watchRuntimeTopics(
          context.runtime,
          { topics: ["mcp.changed", "plugins.changed", "approvals.changed"] },
          signal,
          () => this.refresh(context, signal),
        );
      } catch (error) {
        if (!signal.aborted) {
          this.#snapshot = { ...this.#snapshot, observationFailure: { error } };
          this.#publish();
        }
      }
    });
    return () => controller.abort();
  }
  refresh(context: SettingsContext, signal = context.signal) {
    this.readServers(context, signal);
    this.readRules(context, signal);
    const servers = this.#snapshot.servers.data;
    for (const server of servers ?? [])
      if (this.#snapshot.tools.has(serverKey(server.id)))
        this.readTools(context, server.id, signal);
  }
  readServers(context: SettingsContext, signal = context.signal) {
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
  readRules(context: SettingsContext, signal = context.signal) {
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
    context: SettingsContext,
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
  edit(context: SettingsContext, key: string, draft: MCPServerEdit) {
    this.#assert(context);
    this.#snapshot = {
      ...this.#snapshot,
      drafts: new Map(this.#snapshot.drafts).set(key, draft),
    };
    this.#feedback(key);
    this.#publish();
  }
  save(context: SettingsContext, draft: MCPServerEdit, server?: MCPServerID) {
    this.#assert(context);
    if (!draft.isValid) throw new Error("invalid MCP server draft");
    const input = draft.toInput();
    const update = server ? draft.toUpdate() : undefined;
    if (server && !update) return Promise.resolve(true);
    const key = editorKey(server);
    const target = server ?? userMCPServer(input.name);
    return this.#start(
      context,
      key,
      new CommandBatch([
        {
          resource: serverKey(target),
          execute: (context) =>
            server
              ? context.runtime.mcp.update(updateRequest(server, update!))
              : context.runtime.mcp.create(candidate(input)),
          accept: () => {
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
      ]),
    );
  }
  remove(context: SettingsContext, server: MCPServerID) {
    const key = serverKey(server);
    return this.#start(
      context,
      key,
      new CommandBatch([
        {
          resource: key,
          execute: (context) => context.runtime.mcp.delete(server),
          accept: () => {
            const drafts = new Map(this.#snapshot.drafts);
            drafts.delete(key);
            this.#snapshot = { ...this.#snapshot, drafts };
          },
        },
      ]),
    );
  }
  setEnabled(context: SettingsContext, server: MCPServerID, enabled: boolean) {
    const key = serverKey(server);
    return this.#start(
      context,
      key,
      new CommandBatch([
        {
          resource: key,
          execute: (context) => context.runtime.mcp.update({ server, enabled }),
        },
      ]),
    );
  }
  reconnect(context: SettingsContext, server: MCPServerID) {
    const key = serverKey(server);
    return this.#start(
      context,
      key,
      new CommandBatch([
        {
          resource: key,
          execute: (context) => context.runtime.mcp.reconnect(server),
        },
      ]),
    );
  }
  setExposure(
    context: SettingsContext,
    server: MCPServerID,
    name: string,
    disabled: boolean,
  ) {
    return this.#start(
      context,
      toolPolicyKey(server),
      new CommandBatch([
        {
          resource: serverKey(server),
          execute: (context) =>
            context.runtime.mcp.setToolExposure({ server, name, disabled }),
        },
      ]),
    );
  }
  setAutoApprove(
    context: SettingsContext,
    server: MCPServerID,
    name: string,
    on: boolean,
  ) {
    const rules = this.#snapshot.rules;
    if (rules.phase !== "ready") return Promise.resolve(false);
    const existing = this.autoApproveRule(server, name);
    const resource = serverKey(server);
    const step: CommandStep | undefined = on
      ? {
          resource,
          execute: (context) =>
            context.runtime.approval.setRule({
              tool: { type: "mcp", server, name },
              scope: "global",
              subject: { type: "all" },
              decision: "allow",
            }),
        }
      : existing && {
          resource,
          execute: (context) =>
            context.runtime.approval.forgetRule(existing.id),
        };
    if (!step) return Promise.resolve(true);
    return this.#start(
      context,
      toolPolicyKey(server),
      new CommandBatch([step]),
    );
  }
  autoApproveRule(server: MCPServerID, name: string) {
    return this.#snapshot.rules.data?.find(
      (rule) =>
        rule.tool.type === "mcp" &&
        sameMCPServer(rule.tool.server, server) &&
        rule.tool.name === name &&
        rule.scope === "global" &&
        rule.subject.type === "all" &&
        rule.decision === "allow",
    );
  }
  authorize(context: SettingsContext, server: MCPServerID) {
    const key = serverKey(server);
    return this.#start(
      context,
      key,
      new CommandBatch([
        {
          resource: key,
          execute: (context) =>
            context.runtime.mcp.authorizationAttempts.create(
              server,
              context.signal,
            ),
          accept: (context, value) => {
            const reference = value as MCPAuthorizationAttemptRef;
            const authorization = { server, reference };
            this.#snapshot = {
              ...this.#snapshot,
              authorizations: new Map(this.#snapshot.authorizations).set(
                key,
                authorization,
              ),
            };
            if (!context.signal.aborted)
              this.#poll(context, authorization, context.signal);
          },
        },
      ]),
    );
  }
  async test(context: SettingsContext, key: string, draft: MCPServerEdit) {
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
  setImportText(context: SettingsContext, text: string) {
    this.#assert(context);
    if (this.#snapshot.commands.has(IMPORT)) return;
    this.#import = undefined;
    this.#snapshot = { ...this.#snapshot, importText: text };
    this.#feedback(IMPORT);
    this.#publish();
  }
  importServers(context: SettingsContext) {
    this.#assert(context);
    this.#import ??= new CommandBatch(
      parseMcpImport(this.#snapshot.importText).map((input) => ({
        resource: serverKey(userMCPServer(input.name)),
        execute: (context) => context.runtime.mcp.create(candidate(input)),
      })),
    );
    return this.#start(context, IMPORT, this.#import);
  }
  busy(resource: string): boolean {
    return [...this.#snapshot.commands].some(
      ([key, { batch }]) =>
        key === resource ||
        batch.someRemainingResource((name) => name === resource),
    );
  }
  recover(context: SettingsContext, key: string) {
    this.#assert(context);
    const command = this.#snapshot.commands.get(key);
    if (command?.phase !== "unknown") return Promise.resolve(false);
    return this.#run(context, key, command.batch);
  }
  #start(context: SettingsContext, key: string, batch: CommandBatch) {
    this.#assert(context);
    if (
      batch.complete ||
      this.busy(key) ||
      this.#snapshot.probing.has(key) ||
      batch.someRemainingResource(
        (resource) =>
          this.busy(resource) || this.#snapshot.probing.has(resource),
      )
    )
      return Promise.resolve(false);
    this.#feedback(key);
    return this.#run(context, key, batch);
  }
  async #run(context: SettingsContext, key: string, batch: CommandBatch) {
    const running: ActiveCommand = { phase: "running", batch };
    this.#command(key, running);
    while (!batch.complete) {
      let result: unknown;
      try {
        result = await batch.observe(context);
      } catch (error) {
        if (this.#snapshot.commands.get(key) !== running) return false;
        const observationCanceled =
          context.signal.aborted && error === context.signal.reason;
        if (
          batch.hasOriginal &&
          (mutationSettlementIsUnknown(error) || observationCanceled)
        ) {
          this.#command(key, { phase: "unknown", batch, error });
        } else {
          batch.releaseOriginal();
          if (!observationCanceled)
            this.#feedback(key, { kind: "error", error });
          this.#command(key);
        }
        return false;
      }
      if (this.#snapshot.commands.get(key) !== running) return false;
      batch.accept(context, result);
      if (!batch.complete) this.#publish();
    }
    this.#command(key);
    if (!context.signal.aborted) this.refresh(context);
    return true;
  }
  retryAuthorization(context: SettingsContext, server: MCPServerID) {
    this.#assert(context);
    const authorization = this.#snapshot.authorizations.get(serverKey(server));
    if (authorization && shouldObserveAuthorization(authorization)) {
      this.#feedback(serverKey(server));
      this.#publish();
      this.#poll(context, authorization, context.signal);
    }
  }
  #poll(
    context: SettingsContext,
    authorization: AuthorizationObservation,
    parent: AbortSignal,
  ) {
    const { reference, server } = authorization;
    const previous = this.#polling.get(reference);
    if (previous && !previous.aborted) return;
    const signal = AbortSignal.any([
      parent,
      context.signal,
      this.#source.signal,
    ]);
    this.#polling.set(reference, signal);
    void context.track(async () => {
      let observed = authorization;
      const key = serverKey(server);
      const current = () =>
        this.#snapshot.authorizations.get(key)?.reference === reference;
      try {
        while (shouldObserveAuthorization(observed)) {
          if (observed.attempt) await pollDelay(signal);
          const attempt = await context.runtime.mcp.authorizationAttempts.get(
            reference.attemptId,
            signal,
          );
          if (signal.aborted || !current()) return;
          if (
            attempt.id !== reference.attemptId ||
            !sameMCPServer(attempt.server, server)
          ) {
            throw new Error(
              "MCP authorization status belongs to another attempt",
            );
          }
          observed = { ...authorization, attempt };
          this.#snapshot = {
            ...this.#snapshot,
            authorizations: new Map(this.#snapshot.authorizations).set(
              key,
              observed,
            ),
          };
          this.#publish();
        }
        this.refresh(context, signal);
      } catch (error) {
        if (!signal.aborted && current()) {
          if (isErrorType(error, "mcp_authorization_attempt_not_found")) {
            const authorizations = new Map(this.#snapshot.authorizations);
            authorizations.delete(key);
            this.#snapshot = { ...this.#snapshot, authorizations };
            this.readServers(context, signal);
          }
          this.#feedback(key, { kind: "error", error });
          this.#publish();
        }
      } finally {
        if (this.#polling.get(reference) === signal)
          this.#polling.delete(reference);
      }
    });
  }
  #read<T>(
    context: SettingsContext,
    key: string,
    parent: AbortSignal,
    previous: Read<T> | undefined,
    execute: (signal: AbortSignal) => Promise<T>,
    accept: (read: Read<T>) => void,
  ) {
    if (this.#source.signal.aborted || context.signal.aborted || parent.aborted)
      return;
    this.#assert(context);
    this.#reads.get(key)?.abort();
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
  #assert(context: SettingsContext) {
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
  #command(key: string, value?: ActiveCommand) {
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
    this.#snapshot = {
      ...this.#snapshot,
      importProgress: this.#import?.progress,
    };
    for (const listener of this.#listeners) listener();
  }
}
// The user completes OAuth in their browser; the panel rereads the attempt at a
// pace that notices completion promptly without flooding the Runtime.
const AUTHORIZATION_POLL_MILLISECONDS = 500;

function pollDelay(signal: AbortSignal): Promise<void> {
  signal.throwIfAborted();
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      signal.removeEventListener("abort", abort);
      resolve();
    }, AUTHORIZATION_POLL_MILLISECONDS);
    function abort() {
      clearTimeout(timer);
      reject(signal.reason);
    }
    signal.addEventListener("abort", abort, { once: true });
  });
}

import type { ReviewDecision } from "../application/ports/agentMemoryGateway";
export {
  useAgentMemory,
  addAgentMemory,
  reviewAgentMemory,
  updateAgentMemoryContent,
  setAgentMemoryPinned,
  deleteAgentMemory,
  type AgentMemoryEntry,
  type AgentMemoryQuery,
} from "../application/agentMemoryConfig";

import type { FlameClient } from "@flame/runtime-contract/client";
import {
  CAPABILITY_JSON,
  type MethodInputs,
  type MethodOutputs,
} from "../../../../../../../plugins/memory/io.github.tangerg.flame/contracts/methods.generated";

import type { AgentMemoryEntry } from "../application/workspaceQueries";

export interface MemoryItem extends AgentMemoryEntry {
  sessionId: string;
  day: string;
}

type MemoryPage = MethodOutputs["flame.memory/list.v1"];

export async function readInstalledMemory(
  client: FlameClient,
  scope: "project" | "user",
  cwd: string,
  signal?: AbortSignal,
): Promise<MemoryItem[]> {
  const binding = {
    ...(await selectMemoryBinding(client, signal)),
    contract: CAPABILITY_JSON,
    method: "flame.memory/list.v1",
  };
  const items: MemoryItem[] = [];
  const seen = new Set<string>();
  const cursors = new Set<string>();
  let cursor = "";
  let capacity: number | undefined;
  for (let pageCount = 0; ; pageCount++) {
    signal?.throwIfAborted();
    const reply = await client.plugins.call(
      { ...binding, input: { Scope: scope, CWD: cwd, Cursor: cursor } },
      { signal },
    );
    if (reply.failure) throw new Error(reply.failure.message);
    const page = reply.output as MemoryPage;
    if (
      !page ||
      !Array.isArray(page.Rows) ||
      typeof page.NextCursor !== "string" ||
      !Number.isSafeInteger(page.Capacity) ||
      page.Capacity <= 0 ||
      (capacity !== undefined && capacity !== page.Capacity) ||
      pageCount >= page.Capacity ||
      items.length + page.Rows.length > page.Capacity ||
      (!page.Rows.length && page.NextCursor)
    )
      throw new Error("memory page exceeds its declared traversal bounds");
    capacity = page.Capacity;
    for (const row of page.Rows) {
      if (
        row.Scope !== scope ||
        seen.has(row.ID) ||
        !row.ID ||
        !["active", "pending"].includes(row.Status) ||
        !["auto", "user"].includes(row.Origin)
      )
        throw new Error("memory page has an invalid or repeated item");
      seen.add(row.ID);
      items.push({
        id: row.ID,
        scope,
        content: row.Content,
        origin: row.Origin as MemoryItem["origin"],
        status: row.Status as MemoryItem["status"],
        pinned: row.Pinned,
        sessionId: row.SessionID,
        day: row.Day,
        createdAt: row.CreatedAt,
        updatedAt: row.UpdatedAt,
      });
    }
    if (!page.NextCursor) return items;
    if (cursors.has(page.NextCursor)) throw new Error("memory page repeats a cursor");
    cursors.add(page.NextCursor);
    cursor = page.NextCursor;
  }
}

async function selectMemoryBinding(client: FlameClient, signal?: AbortSignal) {
  const providers = await client.plugins.listProviders(signal);
  const selected = providers.data.find((value) => value.role === "memory");
  if (!selected) throw new Error("memory provider is unavailable");
  const installations = await client.plugins.list(signal);
  const installation = installations.data.find((value) => value.id === selected.installationId);
  if (
    !installation?.enabled ||
    !installation.selected.contributions.some((value) => value.id === selected.contribution)
  )
    throw new Error("selected memory contribution is unavailable");
  return {
    installationId: installation.id,
    digest: installation.selected.digest,
    contribution: selected.contribution,
    target: { scope: "installation", reference: installation.id },
  };
}

export async function deleteInstalledMemory(
  client: FlameClient,
  id: string,
  signal?: AbortSignal,
): Promise<void> {
  const input: MethodInputs["flame.memory/delete.v1"] = { ID: id };
  return client.plugins
    .command(
      `memory-delete:${id}`,
      { contract: CAPABILITY_JSON, method: "flame.memory/delete.v1", input },
      () => selectMemoryBinding(client, signal),
      decodeMemoryAcknowledgement,
      signal,
    )
    .then(() => undefined);
}

export function reviewInstalledMemory(
  client: FlameClient,
  id: string,
  decision: ReviewDecision,
  signal?: AbortSignal,
): Promise<void> {
  const input: MethodInputs["flame.memory/review.v1"] = { Decision: decision, ID: id };
  return client.plugins
    .command(
      `memory-review:${id}`,
      { contract: CAPABILITY_JSON, method: "flame.memory/review.v1", input },
      () => selectMemoryBinding(client, signal),
      decodeMemoryAcknowledgement,
      signal,
    )
    .then(() => undefined);
}

function decodeMemoryAcknowledgement(output: unknown): MethodOutputs["flame.memory/delete.v1"] {
  if (
    !output ||
    typeof output !== "object" ||
    Array.isArray(output) ||
    Object.keys(output).length !== 0
  )
    throw new Error("memory command acknowledgement is invalid");
  return output as MethodOutputs["flame.memory/delete.v1"];
}

export function updateInstalledMemory(
  client: FlameClient,
  input: MethodInputs["flame.memory/update.v1"],
  signal?: AbortSignal,
): Promise<MemoryItem> {
  const patch: MethodInputs["flame.memory/update.v1"] = {
    ...(input.Content === undefined ? {} : { Content: input.Content }),
    ID: input.ID,
    ...(input.Pinned === undefined ? {} : { Pinned: input.Pinned }),
  };
  if (patch.Content === undefined && patch.Pinned === undefined)
    return Promise.reject(new Error("memory update requires a change"));
  return client.plugins.command(
    `memory-update:${patch.ID}`,
    { contract: CAPABILITY_JSON, method: "flame.memory/update.v1", input: patch },
    () => selectMemoryBinding(client, signal),
    (output) => {
      const item = decodeMemoryItem(output);
      if (item.id !== patch.ID)
        throw new Error("memory update acknowledgement changed item identity");
      return item;
    },
    signal,
  );
}

function decodeMemoryItem(output: unknown): MemoryItem {
  const row = output as MethodOutputs["flame.memory/update.v1"];
  if (
    !row ||
    !row.ID ||
    typeof row.Content !== "string" ||
    !row.Content.trim() ||
    typeof row.Pinned !== "boolean" ||
    typeof row.SessionID !== "string" ||
    typeof row.Day !== "string" ||
    !["project", "user"].includes(row.Scope) ||
    !["active", "pending"].includes(row.Status) ||
    !["auto", "user"].includes(row.Origin) ||
    !Number.isFinite(Date.parse(row.CreatedAt)) ||
    !Number.isFinite(Date.parse(row.UpdatedAt)) ||
    Date.parse(row.UpdatedAt) < Date.parse(row.CreatedAt)
  )
    throw new Error("memory item acknowledgement is invalid");
  return {
    id: row.ID,
    scope: row.Scope as "project" | "user",
    content: row.Content,
    origin: row.Origin as "auto" | "user",
    status: row.Status as "active" | "pending",
    pinned: row.Pinned,
    sessionId: row.SessionID,
    day: row.Day,
    createdAt: row.CreatedAt,
    updatedAt: row.UpdatedAt,
  };
}

export function addInstalledMemory(
  client: FlameClient,
  input: { scope: "project" | "user"; cwd?: string; content: string },
  signal?: AbortSignal,
): Promise<MemoryItem> {
  if (input.scope === "project" && !input.cwd)
    return Promise.reject(new Error("project memory addition requires a captured workspace"));
  const request: MethodInputs["flame.memory/add.v1"] = {
    CWD: input.scope === "project" ? input.cwd! : "",
    Content: input.content,
    Scope: input.scope,
  };
  return client.plugins.command(
    "memory-add",
    { contract: CAPABILITY_JSON, method: "flame.memory/add.v1", input: request },
    () => selectMemoryBinding(client, signal),
    (output) => {
      const result = output as MethodOutputs["flame.memory/add.v1"];
      if (!result || typeof result.Changed !== "boolean")
        throw new Error("memory addition acknowledgement is invalid");
      const item = decodeMemoryItem(result.Item);
      if (item.scope !== request.Scope)
        throw new Error("memory addition acknowledgement changed target scope");
      return item;
    },
    signal,
  );
}

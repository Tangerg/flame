import { projectNumber } from "@frame/sdk/json";
import { parseJSON, stringifyJSON } from "@frame/sdk/json";
import { type FlameClient } from "@flame/runtime-client";
import {
  CAPABILITY_JSON,
  type MethodInputs,
  type MethodOutputs,
} from "../../../../plugins/memory/package/io.github.tangerg.flame/contracts/methods.generated";
type MemoryScope = MethodInputs["flame.memory/list.v3"]["scope"];
type MemoryRecord = MethodOutputs["flame.memory/list.v3"]["rows"][number];
type MemoryOrigin = MemoryRecord["origin"];
type MemoryStatus = Exclude<MemoryRecord["status"], "rejected">;
type ReviewDecision = MethodInputs["flame.memory/review.v3"]["decision"];

interface MemoryAddInput {
  scope: MemoryScope;
  cwd?: string;
  content: string;
}

interface MemoryItem {
  id: string;
  revision: number;
  scope: MemoryScope;
  content: string;
  origin: MemoryOrigin;
  status: MemoryStatus;
  pinned: boolean;
  sessionId: string;
  day: string;
  createdAt: string;
  updatedAt: string;
}

type MemoryPage = MethodOutputs["flame.memory/list.v3"];

export async function readInstalledMemory(
  client: FlameClient,
  scope: MemoryScope,
  cwd: string,
  signal?: AbortSignal,
): Promise<MemoryItem[]> {
  const binding = {
    ...(await selectMemoryBinding(client, signal)),
    contract: CAPABILITY_JSON,
    method: "flame.memory/list.v3",
  };
  const items: MemoryItem[] = [];
  const seen = new Set<string>();
  const cursors = new Set<string>();
  let cursor = "";
  let capacity: bigint | undefined;
  for (let pageCount = 0; ; pageCount++) {
    signal?.throwIfAborted();
    const reply = await client.plugins.call(
      {
        ...binding,
        input: stringifyJSON({ scope: scope, cwd: cwd, cursor: cursor }),
      },
      { signal },
    );
    if (reply.failure) throw new Error(reply.failure.message);
    const page = parseJSON(reply.output!) as MemoryPage;
    if (
      !page ||
      (typeof page.capacity !== "bigint" &&
        (typeof page.capacity !== "number" || !Number.isSafeInteger(page.capacity)))
    )
      throw new Error("memory page has an invalid integer capacity");
    const pageCapacity = BigInt(page.capacity);
    if (
      !page ||
      !Array.isArray(page.rows) ||
      typeof page.nextCursor !== "string" ||
      pageCapacity <= 0n ||
      (capacity !== undefined && capacity !== pageCapacity) ||
      BigInt(pageCount) >= pageCapacity ||
      BigInt(items.length + page.rows.length) > pageCapacity ||
      (!page.rows.length && page.nextCursor)
    )
      throw new Error("memory page exceeds its declared traversal bounds");
    capacity = pageCapacity;
    for (const row of page.rows) {
      const revision = projectNumber(row.revision);
      if (
        row.scope !== scope ||
        seen.has(row.id) ||
        !row.id ||
        !Number.isSafeInteger(revision) ||
        revision < 1 ||
        !["active", "pending"].includes(row.status) ||
        !["auto", "user"].includes(row.origin)
      )
        throw new Error("memory page has an invalid or repeated item");
      seen.add(row.id);
      items.push({
        id: row.id,
        revision,
        scope,
        content: row.content,
        origin: row.origin as MemoryItem["origin"],
        status: row.status as MemoryItem["status"],
        pinned: row.pinned,
        sessionId: row.sessionId,
        day: row.day,
        createdAt: row.createdAt,
        updatedAt: row.updatedAt,
      });
    }
    if (!page.nextCursor) return items;
    if (cursors.has(page.nextCursor)) throw new Error("memory page repeats a cursor");
    cursors.add(page.nextCursor);
    cursor = page.nextCursor;
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
  expectedRevision: number,
  signal?: AbortSignal,
): Promise<void> {
  const input: MethodInputs["flame.memory/delete.v3"] = {
    id,
    expectedRevision,
  };
  return client.plugins
    .command(
      `memory-delete:${id}`,
      {
        contract: CAPABILITY_JSON,
        method: "flame.memory/delete.v3",
        input: stringifyJSON(input),
      },
      () => selectMemoryBinding(client, signal),
      decodeMemoryAcknowledgement,
      signal,
    )
    .then(() => undefined);
}

export function reviewInstalledMemory(
  client: FlameClient,
  id: string,
  expectedRevision: number,
  decision: ReviewDecision,
  signal?: AbortSignal,
): Promise<void> {
  const input: MethodInputs["flame.memory/review.v3"] = {
    decision,
    id,
    expectedRevision,
  };
  return client.plugins
    .command(
      `memory-review:${id}`,
      {
        contract: CAPABILITY_JSON,
        method: "flame.memory/review.v3",
        input: stringifyJSON(input),
      },
      () => selectMemoryBinding(client, signal),
      decodeMemoryAcknowledgement,
      signal,
    )
    .then(() => undefined);
}

function decodeMemoryAcknowledgement(output: unknown): MethodOutputs["flame.memory/delete.v3"] {
  if (
    !output ||
    typeof output !== "object" ||
    Array.isArray(output) ||
    Object.keys(output).length !== 0
  )
    throw new Error("memory command acknowledgement is invalid");
  return output as MethodOutputs["flame.memory/delete.v3"];
}

export function updateInstalledMemory(
  client: FlameClient,
  input: MethodInputs["flame.memory/update.v3"],
  signal?: AbortSignal,
): Promise<MemoryItem> {
  const patch: MethodInputs["flame.memory/update.v3"] = {
    ...(input.content === undefined ? {} : { content: input.content }),
    id: input.id,
    expectedRevision: input.expectedRevision,
    ...(input.pinned === undefined ? {} : { pinned: input.pinned }),
  };
  if (patch.content === undefined && patch.pinned === undefined)
    return Promise.reject(new Error("memory update requires a change"));
  return client.plugins.command(
    `memory-update:${patch.id}`,
    {
      contract: CAPABILITY_JSON,
      method: "flame.memory/update.v3",
      input: stringifyJSON(patch),
    },
    () => selectMemoryBinding(client, signal),
    (output) => {
      const item = decodeMemoryItem(output);
      if (item.id !== patch.id)
        throw new Error("memory update acknowledgement changed item identity");
      return item;
    },
    signal,
  );
}

function decodeMemoryItem(output: unknown): MemoryItem {
  const row = output as MethodOutputs["flame.memory/update.v3"];
  const revision = projectNumber(row?.revision);
  if (
    !row ||
    !row.id ||
    !Number.isSafeInteger(revision) ||
    revision < 1 ||
    typeof row.content !== "string" ||
    !row.content.trim() ||
    typeof row.pinned !== "boolean" ||
    typeof row.sessionId !== "string" ||
    typeof row.day !== "string" ||
    !["project", "user"].includes(row.scope) ||
    !["active", "pending"].includes(row.status) ||
    !["auto", "user"].includes(row.origin) ||
    !Number.isFinite(Date.parse(row.createdAt)) ||
    !Number.isFinite(Date.parse(row.updatedAt)) ||
    Date.parse(row.updatedAt) < Date.parse(row.createdAt)
  )
    throw new Error("memory item acknowledgement is invalid");
  return {
    id: row.id,
    revision,
    scope: row.scope as MemoryScope,
    content: row.content,
    origin: row.origin as MemoryOrigin,
    status: row.status as MemoryStatus,
    pinned: row.pinned,
    sessionId: row.sessionId,
    day: row.day,
    createdAt: row.createdAt,
    updatedAt: row.updatedAt,
  };
}

export function addInstalledMemory(
  client: FlameClient,
  input: MemoryAddInput,
  signal?: AbortSignal,
): Promise<MemoryItem> {
  if (input.scope === "project" && !input.cwd)
    return Promise.reject(new Error("project memory addition requires a captured workspace"));
  const request: MethodInputs["flame.memory/add.v3"] = {
    cwd: input.scope === "project" ? input.cwd! : "",
    content: input.content,
    scope: input.scope,
  };
  return client.plugins.command(
    "memory-add",
    {
      contract: CAPABILITY_JSON,
      method: "flame.memory/add.v3",
      input: stringifyJSON(request),
    },
    () => selectMemoryBinding(client, signal),
    (output) => {
      const result = output as MethodOutputs["flame.memory/add.v3"];
      if (!result || typeof result.changed !== "boolean")
        throw new Error("memory addition acknowledgement is invalid");
      const item = decodeMemoryItem(result.item);
      if (item.scope !== request.scope)
        throw new Error("memory addition acknowledgement changed target scope");
      return item;
    },
    signal,
  );
}

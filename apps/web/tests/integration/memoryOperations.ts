import { projectNumber } from "@qimu/sdk/json";
import { parseJSON, stringifyJSON } from "@qimu/sdk/json";
import { type FlameClient } from "@flame/runtime-client";
import {
  CAPABILITY_JSON,
  type MethodInputs,
  type MethodOutputs,
} from "../../../../plugins/memory/package/io.github.tangerg.flame/contracts/methods.generated";
type MemoryScope = MethodInputs["flame.memory/list.v2"]["Scope"];
type MemoryRecord = MethodOutputs["flame.memory/list.v2"]["Rows"][number];
type MemoryOrigin = MemoryRecord["Origin"];
type MemoryStatus = Exclude<MemoryRecord["Status"], "rejected">;
type ReviewDecision = MethodInputs["flame.memory/review.v2"]["Decision"];

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

type MemoryPage = MethodOutputs["flame.memory/list.v2"];

export async function readInstalledMemory(
  client: FlameClient,
  scope: MemoryScope,
  cwd: string,
  signal?: AbortSignal,
): Promise<MemoryItem[]> {
  const binding = {
    ...(await selectMemoryBinding(client, signal)),
    contract: CAPABILITY_JSON,
    method: "flame.memory/list.v2",
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
        input: stringifyJSON({ Scope: scope, CWD: cwd, Cursor: cursor }),
      },
      { signal },
    );
    if (reply.failure) throw new Error(reply.failure.message);
    const page = parseJSON(reply.output!) as MemoryPage;
    if (
      !page ||
      (typeof page.Capacity !== "bigint" &&
        (typeof page.Capacity !== "number" || !Number.isSafeInteger(page.Capacity)))
    )
      throw new Error("memory page has an invalid integer capacity");
    const pageCapacity = BigInt(page.Capacity);
    if (
      !page ||
      !Array.isArray(page.Rows) ||
      typeof page.NextCursor !== "string" ||
      pageCapacity <= 0n ||
      (capacity !== undefined && capacity !== pageCapacity) ||
      BigInt(pageCount) >= pageCapacity ||
      BigInt(items.length + page.Rows.length) > pageCapacity ||
      (!page.Rows.length && page.NextCursor)
    )
      throw new Error("memory page exceeds its declared traversal bounds");
    capacity = pageCapacity;
    for (const row of page.Rows) {
      const revision = projectNumber(row.Revision);
      if (
        row.Scope !== scope ||
        seen.has(row.ID) ||
        !row.ID ||
        !Number.isSafeInteger(revision) ||
        revision < 1 ||
        !["active", "pending"].includes(row.Status) ||
        !["auto", "user"].includes(row.Origin)
      )
        throw new Error("memory page has an invalid or repeated item");
      seen.add(row.ID);
      items.push({
        id: row.ID,
        revision,
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
  expectedRevision: number,
  signal?: AbortSignal,
): Promise<void> {
  const input: MethodInputs["flame.memory/delete.v2"] = {
    ID: id,
    ExpectedRevision: expectedRevision,
  };
  return client.plugins
    .command(
      `memory-delete:${id}`,
      {
        contract: CAPABILITY_JSON,
        method: "flame.memory/delete.v2",
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
  const input: MethodInputs["flame.memory/review.v2"] = {
    Decision: decision,
    ID: id,
    ExpectedRevision: expectedRevision,
  };
  return client.plugins
    .command(
      `memory-review:${id}`,
      {
        contract: CAPABILITY_JSON,
        method: "flame.memory/review.v2",
        input: stringifyJSON(input),
      },
      () => selectMemoryBinding(client, signal),
      decodeMemoryAcknowledgement,
      signal,
    )
    .then(() => undefined);
}

function decodeMemoryAcknowledgement(output: unknown): MethodOutputs["flame.memory/delete.v2"] {
  if (
    !output ||
    typeof output !== "object" ||
    Array.isArray(output) ||
    Object.keys(output).length !== 0
  )
    throw new Error("memory command acknowledgement is invalid");
  return output as MethodOutputs["flame.memory/delete.v2"];
}

export function updateInstalledMemory(
  client: FlameClient,
  input: MethodInputs["flame.memory/update.v2"],
  signal?: AbortSignal,
): Promise<MemoryItem> {
  const patch: MethodInputs["flame.memory/update.v2"] = {
    ...(input.Content === undefined ? {} : { Content: input.Content }),
    ID: input.ID,
    ExpectedRevision: input.ExpectedRevision,
    ...(input.Pinned === undefined ? {} : { Pinned: input.Pinned }),
  };
  if (patch.Content === undefined && patch.Pinned === undefined)
    return Promise.reject(new Error("memory update requires a change"));
  return client.plugins.command(
    `memory-update:${patch.ID}`,
    {
      contract: CAPABILITY_JSON,
      method: "flame.memory/update.v2",
      input: stringifyJSON(patch),
    },
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
  const row = output as MethodOutputs["flame.memory/update.v2"];
  const revision = projectNumber(row?.Revision);
  if (
    !row ||
    !row.ID ||
    !Number.isSafeInteger(revision) ||
    revision < 1 ||
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
    revision,
    scope: row.Scope as MemoryScope,
    content: row.Content,
    origin: row.Origin as MemoryOrigin,
    status: row.Status as MemoryStatus,
    pinned: row.Pinned,
    sessionId: row.SessionID,
    day: row.Day,
    createdAt: row.CreatedAt,
    updatedAt: row.UpdatedAt,
  };
}

export function addInstalledMemory(
  client: FlameClient,
  input: MemoryAddInput,
  signal?: AbortSignal,
): Promise<MemoryItem> {
  if (input.scope === "project" && !input.cwd)
    return Promise.reject(new Error("project memory addition requires a captured workspace"));
  const request: MethodInputs["flame.memory/add.v2"] = {
    CWD: input.scope === "project" ? input.cwd! : "",
    Content: input.content,
    Scope: input.scope,
  };
  return client.plugins.command(
    "memory-add",
    {
      contract: CAPABILITY_JSON,
      method: "flame.memory/add.v2",
      input: stringifyJSON(request),
    },
    () => selectMemoryBinding(client, signal),
    (output) => {
      const result = output as MethodOutputs["flame.memory/add.v2"];
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

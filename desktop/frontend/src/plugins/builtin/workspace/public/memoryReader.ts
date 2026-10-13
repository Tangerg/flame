import type { AgentMemoryItem, FlameClient } from "@flame/runtime-contract/client";
import {
  CAPABILITY_JSON,
  type MethodOutputs,
} from "../../../../../../../plugins/memory/io.github.tangerg.flame/contracts/methods.generated";

const maximumMemoryItems = 512;
const memoryPageWidth = 100;

type MemoryPage = MethodOutputs["flame.memory/list.v1"];

export async function readInstalledMemory(
  client: FlameClient,
  scope: "project" | "user",
  cwd: string,
  signal?: AbortSignal,
): Promise<AgentMemoryItem[]> {
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
  const binding = {
    installationId: installation.id,
    digest: installation.selected.digest,
    contribution: selected.contribution,
    contract: CAPABILITY_JSON,
    method: "flame.memory/list.v1",
    target: { scope: "installation", reference: installation.id },
  };
  const items: AgentMemoryItem[] = [];
  const seen = new Set<string>();
  const cursors = new Set<string>();
  let cursor = "";
  for (let pageCount = 0; pageCount < maximumMemoryItems; pageCount++) {
    signal?.throwIfAborted();
    const reply = await client.plugins.call(
      { ...binding, input: { Scope: scope, CWD: cwd, Cursor: cursor, Limit: memoryPageWidth } },
      { signal },
    );
    if (reply.failure) throw new Error(reply.failure.message);
    const page = reply.output as MemoryPage;
    if (
      !page ||
      !Array.isArray(page.Rows) ||
      typeof page.NextCursor !== "string" ||
      page.Rows.length > memoryPageWidth ||
      items.length + page.Rows.length > maximumMemoryItems ||
      (!page.Rows.length && page.NextCursor)
    )
      throw new Error("memory page exceeds its declared traversal bounds");
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
        origin: row.Origin as AgentMemoryItem["origin"],
        status: row.Status as AgentMemoryItem["status"],
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
  throw new Error("memory traversal exceeds its request bound");
}

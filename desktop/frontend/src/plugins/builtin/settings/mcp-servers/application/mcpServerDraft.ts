import type { MCPServerSettings, MCPTransport } from "./mcpServerConfig";
import type { MCPServerInput } from "./mcpServerInput";
import {
  boundedMCPHandshakeTimeout,
  mcpHandshakeTimeoutSeconds,
  UNBOUNDED_MCP_HANDSHAKE,
  type MCPHandshakeTimeout,
} from "./mcpHandshakeTimeout";

export interface MCPServerDraft {
  name: string;
  transport: MCPTransport;
  description: string;
  command: string;
  args: string;
  environment: RetainedValueDraft;
  dir: string;
  url: string;
  authorization: RetainedValueDraft;
  headers: RetainedValueDraft;
  timeoutSec: string;
  disabledTools: string[];
  autoApproveTools: string[];
}

// Stored credentials are write-only: an empty field means "preserve", while
// the user may explicitly replace or clear the stored value. A discriminated
// union prevents contradictory drafts such as "replace and clear".
export type RetainedValueDraft =
  | { readonly disposition: "preserve" }
  | { readonly disposition: "replace"; readonly text: string }
  | { readonly disposition: "clear" };

function preserveRetainedValue(): RetainedValueDraft {
  return { disposition: "preserve" };
}

export function retainedValueText(value: RetainedValueDraft): string {
  return value.disposition === "replace" ? value.text : "";
}

export function editRetainedValue(text: string): RetainedValueDraft {
  return text.trim() ? { disposition: "replace", text } : preserveRetainedValue();
}

export function setRetainedValueCleared(cleared: boolean): RetainedValueDraft {
  return cleared ? { disposition: "clear" } : preserveRetainedValue();
}

function linesToList(text: string): string[] | undefined {
  const list = text
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
  return list.length ? list : undefined;
}

// Accumulated in a Map and materialised once. Assigning a typed name straight onto
// an object literal silently DROPS an entry called `__proto__` — the setter takes
// the string and stores nothing — so an environment variable by that name would
// vanish between the field and the wire.
function linesToMap(text: string): Record<string, string> | undefined {
  const out = new Map<string, string>();
  for (const raw of text.split("\n")) {
    const line = raw.trim();
    if (!line) continue;
    const i = line.indexOf("=");
    if (i === -1) out.set(line, "");
    else out.set(line.slice(0, i), line.slice(i + 1));
  }
  return out.size ? Object.fromEntries(out) : undefined;
}

export function initialMCPServerDraft(server?: MCPServerSettings): MCPServerDraft {
  return {
    name: server?.name ?? "",
    transport: server?.type ?? "stdio",
    description: server?.description ?? "",
    command: server?.command ?? "",
    args: (server?.args ?? []).join("\n"),
    environment: preserveRetainedValue(),
    dir: server?.dir ?? "",
    url: server?.url ?? "",
    authorization: preserveRetainedValue(),
    headers: preserveRetainedValue(),
    timeoutSec: server ? String(mcpHandshakeTimeoutSeconds(server.handshakeTimeout) ?? "") : "",
    disabledTools: server?.disabledTools ?? [],
    autoApproveTools: server?.autoApproveTools ?? [],
  };
}

export function isMCPServerDraftValid(draft: MCPServerDraft, server?: MCPServerSettings): boolean {
  return (
    draft.name.trim() !== "" &&
    (draft.transport === "stdio" ? draft.command.trim() !== "" : draft.url.trim() !== "") &&
    !mcpAuthorizationNeedsDisposition(draft, server) &&
    !mcpHeadersNeedDisposition(draft, server) &&
    !mcpEnvironmentNeedsDisposition(draft, server) &&
    handshakeTimeoutFromDraft(draft.timeoutSec) !== undefined
  );
}

export function mcpEnvironmentNeedsDisposition(
  draft: MCPServerDraft,
  server?: MCPServerSettings,
): boolean {
  return (
    draft.transport === "stdio" &&
    draft.environment.disposition === "preserve" &&
    server?.type === "stdio" &&
    Boolean(server.envMasked && Object.keys(server.envMasked).length > 0) &&
    !sameStdioTarget(server, draft)
  );
}

export function mcpHeadersNeedDisposition(
  draft: MCPServerDraft,
  server?: MCPServerSettings,
): boolean {
  return (
    draft.transport === "streamableHttp" &&
    draft.headers.disposition === "preserve" &&
    server?.type === "streamableHttp" &&
    Boolean(server.headersMasked && Object.keys(server.headersMasked).length > 0) &&
    !sameHTTPOrigin(server.url, draft.url)
  );
}

export function mcpAuthorizationNeedsDisposition(
  draft: MCPServerDraft,
  server?: MCPServerSettings,
): boolean {
  return (
    draft.transport === "streamableHttp" &&
    draft.authorization.disposition === "preserve" &&
    server?.type === "streamableHttp" &&
    Boolean(server.authorizationMasked) &&
    !sameHTTPOrigin(server.url, draft.url)
  );
}

export function mcpServerInputFromDraft(
  draft: MCPServerDraft,
  server?: MCPServerSettings,
): MCPServerInput {
  const handshakeTimeout = handshakeTimeoutFromDraft(draft.timeoutSec);
  if (handshakeTimeout === undefined) {
    throw new Error("MCP handshake timeout must be blank or a positive integer");
  }
  const base: MCPServerInput = {
    name: draft.name.trim(),
    transport: draft.transport,
    enabled: server?.enabled ?? true,
    description: draft.description.trim() || undefined,
    handshakeTimeout,
    disabledTools: draft.disabledTools.length ? draft.disabledTools : undefined,
    autoApproveTools: draft.autoApproveTools.length ? draft.autoApproveTools : undefined,
  };
  if (draft.transport === "stdio") {
    return {
      ...base,
      command: draft.command.trim() || undefined,
      args: linesToList(draft.args),
      env: environmentFromDraft(draft),
      dir: draft.dir.trim() || undefined,
    };
  }
  return {
    ...base,
    url: draft.url.trim() || undefined,
    authorization: authorizationFromDraft(draft),
    headers: headersFromDraft(draft),
  };
}

function handshakeTimeoutFromDraft(value: string): MCPHandshakeTimeout | undefined {
  const normalized = value.trim();
  if (normalized === "") return UNBOUNDED_MCP_HANDSHAKE;
  if (!/^\d+$/.test(normalized)) return undefined;
  const seconds = Number(normalized);
  try {
    return boundedMCPHandshakeTimeout(seconds);
  } catch {
    return undefined;
  }
}

function authorizationFromDraft(draft: MCPServerDraft): string | null | undefined {
  if (draft.authorization.disposition === "clear") return null;
  if (draft.authorization.disposition === "preserve") return undefined;
  return draft.authorization.text.trim() || undefined;
}

function headersFromDraft(draft: MCPServerDraft): Record<string, string> | null | undefined {
  if (draft.headers.disposition === "clear") return null;
  if (draft.headers.disposition === "preserve") return undefined;
  return linesToMap(draft.headers.text);
}

function environmentFromDraft(draft: MCPServerDraft): Record<string, string> | null | undefined {
  if (draft.environment.disposition === "clear") return null;
  if (draft.environment.disposition === "preserve") return undefined;
  return linesToMap(draft.environment.text);
}

function sameStdioTarget(server: MCPServerSettings, draft: MCPServerDraft): boolean {
  const args = linesToList(draft.args) ?? [];
  const storedArgs = server.args ?? [];
  return (
    server.command === draft.command.trim() &&
    storedArgs.length === args.length &&
    storedArgs.every((value, index) => value === args[index]) &&
    (server.dir ?? "") === draft.dir.trim()
  );
}

function sameHTTPOrigin(left: string | undefined, right: string): boolean {
  try {
    return new URL(left ?? "").origin === new URL(right).origin;
  } catch {
    return false;
  }
}

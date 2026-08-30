// Pasted JSON is an EXTERNAL trust boundary (CLAUDE.md §3), so it is validated with Zod
// before any field is trusted. The accepted shape is LENIENT because different MCP clients
// emit different subsets; `type` is inferred when absent (command ⇒ stdio, url ⇒ http).

import { z } from "zod";
import type { MCPServerInput } from "./mcpServerInput";
import { mcpHandshakeTimeoutFromOptionalSeconds } from "./mcpHandshakeTimeout";

// Split on the FIRST '=', so a value may itself contain one.
const envSchema = z
  .union([z.record(z.string(), z.string()), z.array(z.string())])
  .optional()
  .transform((env) => {
    if (env === undefined) return undefined;
    if (!Array.isArray(env)) return env;
    const out: Record<string, string> = {};
    for (const kv of env) {
      const i = kv.indexOf("=");
      if (i === -1) out[kv] = "";
      else out[kv.slice(0, i)] = kv.slice(i + 1);
    }
    return out;
  });

const serverSchema = z.object({
  // A lenient string, NOT an enum, so a novel type value pastes in rather than failing.
  type: z.string().optional(),
  command: z.string().optional(),
  args: z.array(z.string()).optional(),
  env: envSchema,
  dir: z.string().optional(),
  cwd: z.string().optional(),
  url: z.string().optional(),
  // A bearer token may arrive bare OR as a Headers "Authorization" entry.
  authorization: z.string().optional(),
  headers: z.record(z.string(), z.string()).optional(),
  timeout: z.number().optional(), // seconds; 0/absent = unbounded
});

type ParsedServer = z.infer<typeof serverSchema>;

function authorizationFrom(s: ParsedServer): string | undefined {
  const raw = s.authorization ?? s.headers?.Authorization ?? s.headers?.authorization ?? undefined;
  return raw;
}

// `undefined` when nothing remains, so an Authorization-only block does not store an
// empty map.
function headersExceptAuth(s: ParsedServer): Record<string, string> | undefined {
  if (!s.headers) return undefined;
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(s.headers)) {
    if (k.toLowerCase() === "authorization") continue;
    out[k] = v;
  }
  return Object.keys(out).length ? out : undefined;
}

export interface McpImportResult {
  servers: MCPServerInput[];
}

/**
 * Parse an MCP-client JSON string into configure requests, one per
 * named server. Throws on malformed JSON or a server entry that matches
 * neither transport (no command and no url) — the caller surfaces the message.
 */
export function parseMcpImport(text: string): McpImportResult {
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch {
    throw new Error("Not valid JSON");
  }
  const parsed = z.object({ mcpServers: z.record(z.string(), serverSchema) }).safeParse(raw);
  if (!parsed.success) {
    throw new Error('Expected {"mcpServers": { "<name>": { … } }}');
  }
  const servers: MCPServerInput[] = [];
  for (const [name, s] of Object.entries(parsed.data.mcpServers)) {
    // Every url-based type collapses onto `streamableHttp`, the one remote transport.
    const type = s.type
      ? s.type === "stdio"
        ? "stdio"
        : "streamableHttp"
      : s.command
        ? "stdio"
        : s.url
          ? "streamableHttp"
          : undefined;
    if (type === undefined) {
      throw new Error(`Server "${name}" has neither a command (stdio) nor a url (streamableHttp)`);
    }
    if (type === "stdio") {
      servers.push({
        name,
        transport: type,
        enabled: true,
        command: s.command,
        args: s.args,
        env: s.env,
        dir: s.dir ?? s.cwd,
        handshakeTimeout: mcpHandshakeTimeoutFromOptionalSeconds(s.timeout),
      });
    } else {
      servers.push({
        name,
        transport: type,
        enabled: true,
        url: s.url,
        authorization: authorizationFrom(s),
        headers: headersExceptAuth(s),
        handshakeTimeout: mcpHandshakeTimeoutFromOptionalSeconds(s.timeout),
      });
    }
  }
  if (servers.length === 0) throw new Error("No servers found under mcpServers");
  return { servers };
}

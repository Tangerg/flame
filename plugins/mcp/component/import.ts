import { z } from "zod";
import { type MCPServerInput } from "./input";
import {
  MCPHandshakeTimeoutError,
  mcpHandshakeTimeoutFromOptionalSeconds,
  type MCPHandshakeTimeout,
} from "./timeout";
const envSchema = z
  .union([z.record(z.string(), z.string()), z.array(z.string())])
  .optional()
  .transform((env) => {
    if (env === undefined) return undefined;
    if (!Array.isArray(env)) return env;
    const out = new Map<string, string>();
    for (const kv of env) {
      const i = kv.indexOf("=");
      if (i === -1) out.set(kv, "");
      else out.set(kv.slice(0, i), kv.slice(i + 1));
    }
    return Object.fromEntries(out);
  });

const serverSchema = z.object({
  type: z.string().optional(),
  command: z.string().optional(),
  args: z.array(z.string()).optional(),
  env: envSchema,
  dir: z.string().optional(),
  cwd: z.string().optional(),
  url: z.string().optional(),
  authorization: z.string().optional(),
  headers: z.record(z.string(), z.string()).optional(),
  timeout: z.number().optional(),
});

type ParsedServer = z.infer<typeof serverSchema>;

function authorizationFrom(s: ParsedServer): string | undefined {
  const raw =
    s.authorization ??
    s.headers?.Authorization ??
    s.headers?.authorization ??
    undefined;
  return raw;
}

function headersExceptAuth(
  s: ParsedServer,
): Record<string, string> | undefined {
  if (!s.headers) return undefined;
  const out = new Map<string, string>();
  for (const [k, v] of Object.entries(s.headers)) {
    if (k.toLowerCase() === "authorization") continue;
    out.set(k, v);
  }
  return out.size ? Object.fromEntries(out) : undefined;
}

export interface McpImportResult {
  servers: MCPServerInput[];
  renamed: { from: string; to: string }[];
}

export type McpImportIssue =
  | { readonly kind: "json" }
  | { readonly kind: "shape" }
  | { readonly kind: "name"; readonly server: string }
  | {
      readonly kind: "nameCollision";
      readonly first: string;
      readonly second: string;
      readonly canonical: string;
    }
  | { readonly kind: "transport"; readonly server: string }
  | { readonly kind: "command"; readonly server: string }
  | { readonly kind: "url"; readonly server: string }
  | { readonly kind: "timeout"; readonly server: string }
  | { readonly kind: "empty" };

export class McpImportError extends Error {
  constructor(readonly issue: McpImportIssue) {
    super("invalid_mcp_import");
    this.name = "McpImportError";
  }
}

function handshakeTimeoutOf(
  server: string,
  seconds: number | undefined,
): MCPHandshakeTimeout {
  try {
    return mcpHandshakeTimeoutFromOptionalSeconds(seconds);
  } catch (error) {
    if (error instanceof MCPHandshakeTimeoutError) {
      throw new McpImportError({ kind: "timeout", server });
    }
    throw error;
  }
}

function wireServerName(name: string): string | null {
  const shaped = name
    .toLowerCase()
    .replace(/[^a-z0-9._-]+/g, "-")
    .replace(/^[^a-z0-9]+/, "")
    .slice(0, 32)
    .replace(/[-._]+$/, "");
  return /^[a-z0-9][a-z0-9._-]{0,31}$/.test(shaped) ? shaped : null;
}

export function parseMcpImport(text: string): McpImportResult {
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch (error) {
    if (error instanceof SyntaxError)
      throw new McpImportError({ kind: "json" });
    throw error;
  }
  const parsed = z
    .object({ mcpServers: z.record(z.string(), serverSchema) })
    .safeParse(raw);
  if (!parsed.success) throw new McpImportError({ kind: "shape" });
  const servers: MCPServerInput[] = [];
  const renamed: { from: string; to: string }[] = [];
  const sources = new Map<string, string>();
  for (const [key, s] of Object.entries(parsed.data.mcpServers)) {
    const name = wireServerName(key);
    if (name === null) throw new McpImportError({ kind: "name", server: key });
    const earlier = sources.get(name);
    if (earlier !== undefined) {
      throw new McpImportError({
        kind: "nameCollision",
        first: earlier,
        second: key,
        canonical: name,
      });
    }
    sources.set(name, key);
    if (name !== key) renamed.push({ from: key, to: name });
    const type = s.type
      ? s.type === "stdio"
        ? "stdio"
        : s.type === "http" || s.type === "streamableHttp" || s.type === "sse"
          ? "streamableHttp"
          : undefined
      : s.command
        ? "stdio"
        : s.url
          ? "streamableHttp"
          : undefined;
    if (type === undefined)
      throw new McpImportError({ kind: "transport", server: key });
    if (type === "stdio") {
      if (!s.command?.trim())
        throw new McpImportError({ kind: "command", server: key });
      servers.push({
        name,
        transport: type,
        enabled: true,
        command: s.command,
        args: s.args,
        env: s.env,
        dir: s.dir ?? s.cwd,
        handshakeTimeout: handshakeTimeoutOf(key, s.timeout),
      });
    } else {
      if (!s.url?.trim())
        throw new McpImportError({ kind: "url", server: key });
      servers.push({
        name,
        transport: type,
        enabled: true,
        url: s.url,
        authorization: authorizationFrom(s),
        headers: headersExceptAuth(s),
        handshakeTimeout: handshakeTimeoutOf(key, s.timeout),
      });
    }
  }
  if (servers.length === 0) throw new McpImportError({ kind: "empty" });
  return { servers, renamed };
}

import { z } from "zod";
import { parseJSON } from "@qimu/sdk/json";
import { validateWire } from "@flame/runtime-client/validation";
import { type MCPTransport } from "./model";
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
  timeout: z.unknown().optional(),
});

type ParsedServer = z.infer<typeof serverSchema>;

function credentialsOf(server: string, source: ParsedServer) {
  let authorization = source.authorization;
  const seen = new Set<string>();
  const headers = new Map<string, string>();
  for (const [key, value] of Object.entries(source.headers ?? {})) {
    const field = key.toLowerCase();
    if (
      seen.has(field) ||
      (field === "authorization" && authorization !== undefined)
    ) {
      throw new McpImportError({
        kind: "headerConflict",
        server,
        header: field,
      });
    }
    seen.add(field);
    if (field === "authorization") authorization = value;
    else headers.set(key, value);
  }
  return {
    authorization,
    headers: headers.size ? Object.fromEntries(headers) : undefined,
  };
}

export type McpImportIssue =
  | { readonly kind: "json" }
  | { readonly kind: "shape" }
  | { readonly kind: "name"; readonly server: string }
  | {
      readonly kind: "headerConflict";
      readonly server: string;
      readonly header: string;
    }
  | { readonly kind: "unsupportedTransport"; readonly server: string }
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
  seconds: unknown,
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

function transportOf(server: string, source: ParsedServer): MCPTransport {
  if (
    source.type !== undefined &&
    validateWire("MCPTransport", source.type, "request").length
  ) {
    throw new McpImportError({ kind: "unsupportedTransport", server });
  }
  const hasCommand = source.command !== undefined;
  const hasURL = source.url !== undefined;
  if (
    (hasCommand && hasURL) ||
    (source.type === undefined && !hasCommand && !hasURL)
  ) {
    throw new McpImportError({ kind: "transport", server });
  }
  return (
    (source.type as MCPTransport | undefined) ??
    (hasCommand ? "stdio" : "streamableHttp")
  );
}

export function parseMcpImport(text: string): MCPServerInput[] {
  let raw: unknown;
  try {
    raw = parseJSON(text);
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
  for (const [name, s] of Object.entries(parsed.data.mcpServers)) {
    if (
      validateWire("MCPServerID", { origin: { type: "user" }, name }, "request")
        .length
    ) {
      throw new McpImportError({ kind: "name", server: name });
    }
    const type = transportOf(name, s);
    if (type === "stdio") {
      if (s.command === undefined || s.command === "") {
        throw new McpImportError({ kind: "command", server: name });
      }
      servers.push({
        name,
        transport: type,
        enabled: true,
        command: s.command,
        args: s.args,
        env: s.env,
        dir: s.dir ?? s.cwd,
        handshakeTimeout: handshakeTimeoutOf(name, s.timeout),
      });
    } else if (type === "streamableHttp") {
      if (!s.url?.trim())
        throw new McpImportError({ kind: "url", server: name });
      servers.push({
        name,
        transport: type,
        enabled: true,
        url: s.url,
        ...credentialsOf(name, s),
        handshakeTimeout: handshakeTimeoutOf(name, s.timeout),
      });
    } else {
      throw new McpImportError({ kind: "unsupportedTransport", server: name });
    }
  }
  if (servers.length === 0) throw new McpImportError({ kind: "empty" });
  return servers;
}

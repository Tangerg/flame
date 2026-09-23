import { z } from "zod";
import type { MCPServerInput } from "./mcpServerInput";
import { mcpHandshakeTimeoutFromOptionalSeconds } from "./mcpHandshakeTimeout";

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
  const raw = s.authorization ?? s.headers?.Authorization ?? s.headers?.authorization ?? undefined;
  return raw;
}

function headersExceptAuth(s: ParsedServer): Record<string, string> | undefined {
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
  } catch {
    throw new Error("Not valid JSON");
  }
  const parsed = z.object({ mcpServers: z.record(z.string(), serverSchema) }).safeParse(raw);
  if (!parsed.success) {
    throw new Error('Expected {"mcpServers": { "<name>": { … } }}');
  }
  const servers: MCPServerInput[] = [];
  const renamed: { from: string; to: string }[] = [];
  for (const [key, s] of Object.entries(parsed.data.mcpServers)) {
    const name = wireServerName(key);
    if (name === null) {
      throw new Error(`Server "${key}" has no name the runtime can accept`);
    }
    if (name !== key) renamed.push({ from: key, to: name });
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
      throw new Error(`Server "${key}" has neither a command (stdio) nor a url (streamableHttp)`);
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
  return { servers, renamed };
}

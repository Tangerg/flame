import type { MCPServerSettings, MCPTransport } from "./mcpServerConfig";
import type { MCPServerInput } from "./mcpServerInput";
import {
  boundedMCPHandshakeTimeout,
  mcpHandshakeTimeoutSeconds,
  UNBOUNDED_MCP_HANDSHAKE,
  type MCPHandshakeTimeout,
} from "./mcpHandshakeTimeout";

/**
 * A stored credential the server will not read back.
 *
 * Three states rather than a string plus a flag: "replace and clear" has to be
 * unconstructable, and an empty field has to mean "leave it alone" rather than "erase it".
 */
export class RetainedValue {
  private static readonly PRESERVED = new RetainedValue({ disposition: "preserve" });

  private constructor(
    private readonly state:
      | { readonly disposition: "preserve" }
      | { readonly disposition: "replace"; readonly text: string }
      | { readonly disposition: "clear" },
  ) {}

  static preserved(): RetainedValue {
    return RetainedValue.PRESERVED;
  }

  get disposition(): "preserve" | "replace" | "clear" {
    return this.state.disposition;
  }

  get text(): string {
    return this.state.disposition === "replace" ? this.state.text : "";
  }

  /** Blank is not a replacement — it is the field left untouched. */
  edited(text: string): RetainedValue {
    return text.trim()
      ? new RetainedValue({ disposition: "replace", text })
      : RetainedValue.PRESERVED;
  }

  cleared(clear: boolean): RetainedValue {
    return clear ? new RetainedValue({ disposition: "clear" }) : RetainedValue.PRESERVED;
  }

  /** `null` erases, `undefined` leaves the stored value alone — the wire's distinction. */
  submittedText(): string | null | undefined {
    if (this.state.disposition === "clear") return null;
    if (this.state.disposition === "preserve") return undefined;
    return this.state.text.trim() || undefined;
  }

  submittedPairs(
    parse: (text: string) => Record<string, string> | undefined,
  ): Record<string, string> | null | undefined {
    if (this.state.disposition === "clear") return null;
    if (this.state.disposition === "preserve") return undefined;
    return parse(this.state.text);
  }
}

/** What the form binds to. Every rule about it belongs to `MCPServerEdit`. */
export interface MCPServerFields {
  name: string;
  transport: MCPTransport;
  description: string;
  command: string;
  args: string;
  environment: RetainedValue;
  dir: string;
  url: string;
  authorization: RetainedValue;
  headers: RetainedValue;
  timeoutSec: string;
  disabledTools: string[];
  autoApproveTools: string[];
}

/**
 * One edit in progress against the server it edits.
 *
 * The stored server is a MEMBER, not a parameter: validity and every disposition rule are
 * questions about the pair, and passing it to five free functions let a caller ask one of
 * them about a different server than the others. A masked secret plus a changed target is
 * the whole of those rules — the runtime will not read the secret back, so re-pointing a
 * server at a new target without saying what becomes of it is not an answerable request.
 */
export class MCPServerEdit {
  private constructor(
    readonly fields: MCPServerFields,
    private readonly stored: MCPServerSettings | undefined,
  ) {}

  static of(server?: MCPServerSettings): MCPServerEdit {
    return new MCPServerEdit(
      {
        name: server?.name ?? "",
        transport: server?.type ?? "stdio",
        description: server?.description ?? "",
        command: server?.command ?? "",
        args: (server?.args ?? []).join("\n"),
        environment: RetainedValue.preserved(),
        dir: server?.dir ?? "",
        url: server?.url ?? "",
        authorization: RetainedValue.preserved(),
        headers: RetainedValue.preserved(),
        timeoutSec: server ? String(mcpHandshakeTimeoutSeconds(server.handshakeTimeout) ?? "") : "",
        disabledTools: server?.disabledTools ?? [],
        autoApproveTools: server?.autoApproveTools ?? [],
      },
      server,
    );
  }

  with<K extends keyof MCPServerFields>(key: K, value: MCPServerFields[K]): MCPServerEdit {
    return new MCPServerEdit({ ...this.fields, [key]: value }, this.stored);
  }

  /** The tool lists move together — a per-key call would publish a half-applied selection. */
  withToolSelection(selection: Pick<MCPServerFields, "disabledTools" | "autoApproveTools">) {
    return new MCPServerEdit({ ...this.fields, ...selection }, this.stored);
  }

  get isValid(): boolean {
    const { name, transport, command, url } = this.fields;
    return (
      name.trim() !== "" &&
      (transport === "stdio" ? command.trim() !== "" : url.trim() !== "") &&
      !this.authorizationNeedsDisposition &&
      !this.headersNeedDisposition &&
      !this.environmentNeedsDisposition &&
      this.handshakeTimeout !== undefined
    );
  }

  get environmentNeedsDisposition(): boolean {
    return (
      this.fields.transport === "stdio" &&
      this.fields.environment.disposition === "preserve" &&
      this.stored?.type === "stdio" &&
      Boolean(this.stored.envMasked && Object.keys(this.stored.envMasked).length > 0) &&
      !this.sameStdioTarget(this.stored)
    );
  }

  get headersNeedDisposition(): boolean {
    return (
      this.fields.transport === "streamableHttp" &&
      this.fields.headers.disposition === "preserve" &&
      this.stored?.type === "streamableHttp" &&
      Boolean(this.stored.headersMasked && Object.keys(this.stored.headersMasked).length > 0) &&
      !sameHTTPOrigin(this.stored.url, this.fields.url)
    );
  }

  get authorizationNeedsDisposition(): boolean {
    return (
      this.fields.transport === "streamableHttp" &&
      this.fields.authorization.disposition === "preserve" &&
      this.stored?.type === "streamableHttp" &&
      Boolean(this.stored.authorizationMasked) &&
      !sameHTTPOrigin(this.stored.url, this.fields.url)
    );
  }

  /** Throws rather than coercing: `isValid` already answered, and a silent fallback would
   *  save a timeout the person did not ask for. */
  toInput(): MCPServerInput {
    const handshakeTimeout = this.handshakeTimeout;
    if (handshakeTimeout === undefined) {
      throw new Error("MCP handshake timeout must be blank or a positive integer");
    }
    const f = this.fields;
    const base: MCPServerInput = {
      name: f.name.trim(),
      transport: f.transport,
      enabled: this.stored?.enabled ?? true,
      description: f.description.trim() || undefined,
      handshakeTimeout,
      disabledTools: f.disabledTools.length ? f.disabledTools : undefined,
      autoApproveTools: f.autoApproveTools.length ? f.autoApproveTools : undefined,
    };
    if (f.transport === "stdio") {
      return {
        ...base,
        command: f.command.trim() || undefined,
        args: linesToList(f.args),
        env: f.environment.submittedPairs(linesToMap),
        dir: f.dir.trim() || undefined,
      };
    }
    return {
      ...base,
      url: f.url.trim() || undefined,
      authorization: f.authorization.submittedText(),
      headers: f.headers.submittedPairs(linesToMap),
    };
  }

  private get handshakeTimeout(): MCPHandshakeTimeout | undefined {
    const normalized = this.fields.timeoutSec.trim();
    if (normalized === "") return UNBOUNDED_MCP_HANDSHAKE;
    if (!/^\d+$/.test(normalized)) return undefined;
    try {
      return boundedMCPHandshakeTimeout(Number(normalized));
    } catch {
      return undefined;
    }
  }

  private sameStdioTarget(server: MCPServerSettings): boolean {
    const args = linesToList(this.fields.args) ?? [];
    const storedArgs = server.args ?? [];
    return (
      server.command === this.fields.command.trim() &&
      storedArgs.length === args.length &&
      storedArgs.every((value, index) => value === args[index]) &&
      (server.dir ?? "") === this.fields.dir.trim()
    );
  }
}

function linesToList(text: string): string[] | undefined {
  const list = text
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
  return list.length ? list : undefined;
}

// Accumulated in a Map and materialised once: an entry named `__proto__` assigns nothing to
// an object literal, so the pair would vanish silently.
function linesToMap(text: string): Record<string, string> | undefined {
  const out = new Map<string, string>();
  for (const line of text.split("\n")) {
    const kv = line.trim();
    if (!kv) continue;
    const i = kv.indexOf("=");
    if (i === -1) out.set(kv, "");
    else out.set(kv.slice(0, i).trim(), kv.slice(i + 1).trim());
  }
  return out.size ? Object.fromEntries(out) : undefined;
}

function sameHTTPOrigin(left: string | undefined, right: string): boolean {
  try {
    return new URL(left ?? "").origin === new URL(right).origin;
  } catch {
    return false;
  }
}

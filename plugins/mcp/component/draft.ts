import { type MCPServerSettings } from "./model";
import { type MCPTransport } from "./model";
import {
  type MCPConnectionInput,
  type MCPServerInput,
  type MCPServerUpdateInput,
} from "./input";
import { parseJSON } from "@qimu/sdk/json";
import { validateWire } from "@flame/runtime-client/validation";
import {
  boundedMCPHandshakeTimeout,
  MCPHandshakeTimeoutError,
  mcpHandshakeTimeoutSeconds,
  UNBOUNDED_MCP_HANDSHAKE,
  type MCPHandshakeTimeout,
} from "./timeout";
export class RetainedValue {
  private static readonly PRESERVED = new RetainedValue({
    disposition: "preserve",
  });

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

  edited(text: string): RetainedValue {
    return text.trim()
      ? new RetainedValue({ disposition: "replace", text })
      : RetainedValue.PRESERVED;
  }

  cleared(clear: boolean): RetainedValue {
    return clear
      ? new RetainedValue({ disposition: "clear" })
      : RetainedValue.PRESERVED;
  }

  submittedText(): string | null | undefined {
    if (this.state.disposition === "clear") return null;
    if (this.state.disposition === "preserve") return undefined;
    return this.state.text;
  }

  submittedPairs(
    parse: (text: string) => Record<string, string>,
  ): Record<string, string> | null | undefined {
    if (this.state.disposition === "clear") return null;
    if (this.state.disposition === "preserve") return undefined;
    return parse(this.state.text);
  }
}

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
}

export class MCPServerEdit {
  private constructor(
    readonly fields: MCPServerFields,
    private readonly stored: MCPServerSettings | undefined,
  ) {}

  static of(server?: MCPServerSettings): MCPServerEdit {
    return new MCPServerEdit(
      {
        name: server?.id.name ?? "",
        transport: server?.type ?? "stdio",
        description: server?.description ?? "",
        command: server?.command ?? "",
        args: server ? JSON.stringify(server.args ?? [], null, 2) : "",
        environment: RetainedValue.preserved(),
        dir: server?.dir ?? "",
        url: server?.url ?? "",
        authorization: RetainedValue.preserved(),
        headers: RetainedValue.preserved(),
        timeoutSec: server
          ? String(mcpHandshakeTimeoutSeconds(server.handshakeTimeout) ?? "")
          : "",
      },
      server,
    );
  }

  with<K extends keyof MCPServerFields>(
    key: K,
    value: MCPServerFields[K],
  ): MCPServerEdit {
    return new MCPServerEdit({ ...this.fields, [key]: value }, this.stored);
  }

  get isValid(): boolean {
    const { name, transport, command, url } = this.fields;
    return (
      name.trim() !== "" &&
      (transport === "stdio" ? command !== "" : url.trim() !== "") &&
      this.jsonIssue === undefined &&
      !this.authorizationNeedsDisposition &&
      !this.headersNeedDisposition &&
      !this.environmentNeedsDisposition &&
      this.handshakeTimeout !== undefined
    );
  }

  get environmentNeedsDisposition(): boolean {
    if (this.jsonIssue === "args") return false;
    return (
      this.fields.transport === "stdio" &&
      this.fields.environment.disposition === "preserve" &&
      this.stored?.type === "stdio" &&
      Boolean(
        this.stored.envMasked && Object.keys(this.stored.envMasked).length > 0,
      ) &&
      !this.sameStdioTarget(this.stored)
    );
  }

  get headersNeedDisposition(): boolean {
    return (
      this.fields.transport === "streamableHttp" &&
      this.fields.headers.disposition === "preserve" &&
      this.stored?.type === "streamableHttp" &&
      Boolean(
        this.stored.headersMasked &&
        Object.keys(this.stored.headersMasked).length > 0,
      ) &&
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

  toInput(): MCPServerInput {
    const handshakeTimeout = this.handshakeTimeout;
    if (handshakeTimeout === undefined) {
      throw new Error(
        "mcp handshake timeout must be blank or a positive integer",
      );
    }
    return {
      ...this.connectionInput(),
      name: this.fields.name.trim(),
      enabled: this.stored?.enabled ?? true,
      description: this.fields.description.trim() || undefined,
      handshakeTimeout,
    };
  }

  toUpdate(): MCPServerUpdateInput | undefined {
    if (!this.stored) throw new Error("mcp_update_requires_inspected_server");
    const input = this.toInput();
    const update: MCPServerUpdateInput = {};
    if ((input.description ?? "") !== (this.stored.description ?? "")) {
      update.description = input.description ?? "";
    }
    if (
      mcpHandshakeTimeoutSeconds(input.handshakeTimeout) !==
      mcpHandshakeTimeoutSeconds(this.stored.handshakeTimeout)
    ) {
      update.handshakeTimeout = input.handshakeTimeout;
    }
    const connectionChanged =
      input.transport !== this.stored.type ||
      (input.transport === "stdio"
        ? !this.sameStdioTarget(this.stored) ||
          this.fields.environment.disposition !== "preserve"
        : input.url !== this.stored.url ||
          this.fields.authorization.disposition !== "preserve" ||
          this.fields.headers.disposition !== "preserve");
    if (connectionChanged) update.connection = this.connectionInput();
    return Object.keys(update).length > 0 ? update : undefined;
  }

  private connectionInput(): MCPConnectionInput {
    const f = this.fields;
    const json = this.jsonFields();
    if (f.transport === "stdio") {
      return {
        transport: f.transport,
        command: f.command,
        args: json.args,
        env: json.env,
        dir: f.dir || undefined,
      };
    }
    return {
      transport: f.transport,
      url: f.url.trim() || undefined,
      authorization: f.authorization.submittedText(),
      headers: json.headers,
    };
  }

  get handshakeTimeout(): MCPHandshakeTimeout | undefined {
    const normalized = this.fields.timeoutSec.trim();
    if (normalized === "") return UNBOUNDED_MCP_HANDSHAKE;
    if (!/^\d+$/.test(normalized)) return undefined;
    try {
      return boundedMCPHandshakeTimeout(Number(normalized));
    } catch (error) {
      if (error instanceof MCPHandshakeTimeoutError) return undefined;
      throw error;
    }
  }

  private sameStdioTarget(server: MCPServerSettings): boolean {
    const args = argumentsFrom(this.fields.args);
    const storedArgs = server.args ?? [];
    return (
      server.command === this.fields.command &&
      storedArgs.length === args.length &&
      storedArgs.every((value, index) => value === args[index]) &&
      (server.dir ?? "") === this.fields.dir
    );
  }

  get jsonIssue(): JSONField | undefined {
    try {
      this.jsonFields();
      return undefined;
    } catch (error) {
      if (error instanceof MCPFieldJSONError) return error.field;
      throw error;
    }
  }

  private jsonFields(): Pick<MCPServerInput, "args" | "env" | "headers"> {
    const f = this.fields;
    return f.transport === "stdio"
      ? {
          args: argumentsFrom(f.args),
          env: f.environment.submittedPairs((text) =>
            stringMapFrom(text, "environment"),
          ),
        }
      : {
          headers: f.headers.submittedPairs((text) =>
            stringMapFrom(text, "headers"),
          ),
        };
  }
}

type JSONField = "args" | "environment" | "headers";

class MCPFieldJSONError extends Error {
  constructor(readonly field: JSONField) {
    super("mcp_form_json_invalid");
  }
}

function fieldJSON(source: string, field: JSONField): unknown {
  try {
    return parseJSON(source);
  } catch (error) {
    if (error instanceof SyntaxError) throw new MCPFieldJSONError(field);
    throw error;
  }
}

function argumentsFrom(source: string): string[] {
  if (source.trim() === "") return [];
  const value = fieldJSON(source, "args");
  if (
    !Array.isArray(value) ||
    !value.every((argument) => typeof argument === "string")
  ) {
    throw new MCPFieldJSONError("args");
  }
  return value;
}

function stringMapFrom(
  source: string,
  field: "environment" | "headers",
): Record<string, string> {
  const value = fieldJSON(source, field);
  const type =
    field === "environment" ? "MCPEnvironmentChange" : "MCPHeadersChange";
  if (validateWire(type, { type: "set", value }, "request").length > 0) {
    throw new MCPFieldJSONError(field);
  }
  return value as Record<string, string>;
}

function sameHTTPOrigin(left: string | undefined, right: string): boolean {
  try {
    return new URL(left ?? "").origin === new URL(right).origin;
  } catch {
    return false;
  }
}

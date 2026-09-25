import {
  commandToolResult,
  patchToolResult,
  searchToolResult,
  webSearchToolResult,
} from "@/plugins/sdk";
import type {
  AgentItem,
  AgentItemStatus,
  AgentMessagePart,
  AgentQuestion,
  AgentToolInvocation,
} from "@/plugins/sdk";
import type { BlockStatus, ContentBlock, QuestionItem } from "@/plugins/sdk/types/contentBlock";
import type { ToolCall, ToolCallStatus } from "@/plugins/sdk/types/agentSessionView";
import { toolCategory } from "../../domain/toolCategory";
import { parseUnifiedDiff } from "../../domain/unifiedDiff";

export function itemStartedAt(item: AgentItem): string {
  return item.type === "toolCall" ? item.startedAt : item.createdAt;
}

export function blockStatus(status: AgentItemStatus): BlockStatus {
  if (status === "running") return "running";
  if (status === "incomplete") return "incomplete";
  return "complete";
}

export function contentText(blocks: AgentMessagePart[] | undefined): string {
  return (blocks ?? [])
    .filter((b): b is Extract<AgentMessagePart, { type: "text" }> => b.type === "text")
    .map((b) => b.text)
    .join("");
}

export function userContentBlocks(content: AgentMessagePart[]): ContentBlock[] {
  const blocks: ContentBlock[] = [];
  const text = contentText(content);
  if (text) blocks.push({ kind: "text", text, status: "complete" });
  for (const b of content) {
    if (b.type === "image") blocks.push({ kind: "image", mime: b.mime, data: b.data });
  }
  return blocks;
}

export function mapQuestion(q: AgentQuestion): QuestionItem[] {
  return q.fields.map((f) =>
    f.type === "choice"
      ? {
          type: "choice" as const,
          prompt: f.prompt,
          header: f.header ?? "",
          options: f.options.map((o) => ({
            label: o.label,
            description: o.description ?? "",
            preview: o.preview,
          })),
          multiple: !!f.multiple,
          allowCustom: !!f.allowCustom,
        }
      : {
          type: "text" as const,
          prompt: f.prompt,
          header: f.header ?? "",
        },
  );
}

export function mapQuestionAnswers(q: AgentQuestion): string[][] | undefined {
  return q.answers?.map((values) => [...values]);
}

function asRecord(v: unknown): Record<string, unknown> | undefined {
  return typeof v === "object" && v !== null && !Array.isArray(v)
    ? (v as Record<string, unknown>)
    : undefined;
}
function asString(v: unknown): string | undefined {
  return typeof v === "string" ? v : undefined;
}
function asNumber(v: unknown): number | undefined {
  return typeof v === "number" && Number.isFinite(v) ? v : undefined;
}
function asArrayLength(v: unknown): number | undefined {
  return Array.isArray(v) ? v.length : undefined;
}

function editChanges(result: unknown): unknown[] {
  const changes = patchToolResult(result)?.changes;
  return Array.isArray(changes) ? changes : [];
}

function firstLine(v: unknown): string | undefined {
  const s = asString(v)?.trim();
  return s ? s.split("\n", 1)[0] : undefined;
}

function nameLabel(tool: AgentToolInvocation): string | undefined {
  const a = tool.arguments;
  switch (tool.name) {
    case "lsp": {
      const op = asString(a.operation);
      if (op === "workspace_symbols") return asString(a.query);
      const path = asString(a.path);
      if (op === "document_symbols" || op === "diagnostics") return path;
      return path ? `${path}:${a.line ?? "?"}:${a.character ?? "?"}` : undefined;
    }
    case "ask_user": {
      const first = Array.isArray(a.questions) ? asRecord(a.questions[0]) : undefined;
      return firstLine(first?.question);
    }
    case "read_shell_output":
    case "stop_shell":
      return asString(a.shell_id);
    case "load_skill":
    case "propose_skill":
      return asString(a.name);
    case "read_skill_resource": {
      const name = asString(a.name);
      const path = asString(a.path);
      return name && path ? `${name}/${path}` : (name ?? path);
    }
    case "search_memory":
    case "search_tools":
      return asString(a.query);
    case "web_fetch":
    case "http_request":
      return asString(a.url);
    default:
      return undefined;
  }
}

export function toolLabel(tool: AgentToolInvocation): string {
  return labelSource(tool).text;
}

export function toolLabelKind(tool: AgentToolInvocation): "path" | "text" {
  return labelSource(tool).path ? "path" : "text";
}

function oneLine(text: string): string {
  if (!text.includes("\n")) return text;
  const first = text.split("\n").find((line) => line.trim() !== "");
  return first?.trim() ?? text.replace(/\s+/g, " ").trim();
}

function labelSource(tool: AgentToolInvocation): { text: string; path: boolean } {
  const source = rawLabelSource(tool);
  return { text: oneLine(source.text), path: source.path };
}

function rawLabelSource(tool: AgentToolInvocation): { text: string; path: boolean } {
  const byName = nameLabel(tool);
  if (byName) return { text: byName, path: false };
  const a = tool.arguments ?? {};
  switch (toolCategory(tool.name)) {
    case "command":
      return {
        text: asString(a.description) || asString(a.command) || tool.name || "command",
        path: false,
      };
    case "fileEdit": {
      const path = asString(a.path);
      if (path) return { text: path, path: true };
      const single = asString(asRecord(editChanges(tool.result)[0])?.path);
      return single === undefined ? { text: tool.name, path: false } : { text: single, path: true };
    }
    case "search":
      return { text: asString(a.query) || asString(a.pattern) || "search", path: false };
    case "webSearch":
      return { text: asString(a.query) || "search", path: false };
    case "read": {
      const path = asString(a.path);
      return path === undefined ? { text: tool.name, path: false } : { text: path, path: true };
    }
    case "subagent":
      return {
        text: asString(a.summary) || firstLine(a.instructions) || tool.name,
        path: false,
      };
    default:
      return { text: tool.name || "tool", path: false };
  }
}

export function toolFields(tool: AgentToolInvocation): Partial<ToolCall> {
  const result = asRecord(tool.result);
  const operation = asString(tool.arguments.operation);
  return {
    ...(operation !== undefined ? { operation } : {}),
    ...categoryFields(tool, result),
  };
}

function rawResult(result: unknown): Pick<ToolCall, "result"> | Record<string, never> {
  return result === undefined
    ? {}
    : { result: typeof result === "string" ? result : JSON.stringify(result) };
}

function categoryFields(
  tool: AgentToolInvocation,
  result: Record<string, unknown> | undefined,
): Partial<ToolCall> {
  switch (toolCategory(tool.name)) {
    case "command": {
      const merged = asString(commandToolResult(tool.result)?.output) ?? asString(tool.result);
      return {
        exitCode: asNumber(commandToolResult(tool.result)?.exitCode),
        ...(asString(tool.arguments?.command) !== undefined
          ? { command: asString(tool.arguments?.command) }
          : {}),
        ...(merged !== undefined ? { result: merged } : {}),
      };
    }
    case "fileEdit": {
      const proposed = parseUnifiedDiff(asString(tool.arguments?.patch) ?? "");
      const changes = editChanges(tool.result);
      const files = changes.length || proposed.length;
      return {
        ...(files > 1 ? { files } : {}),
        ...(proposed.length > 0
          ? {
              changes: proposed,
              added: proposed.reduce((sum, file) => sum + file.added, 0),
              removed: proposed.reduce((sum, file) => sum + file.removed, 0),
            }
          : {}),
        ...rawResult(tool.result),
      };
    }
    case "search":
      return {
        hits: asArrayLength(searchToolResult(tool.result)?.hits),
        ...rawResult(tool.result),
      };
    case "webSearch":
      return {
        hits: asArrayLength(webSearchToolResult(tool.result)?.results),
        ...rawResult(tool.result),
      };
    case "read": {
      const content = asString(result?.content);
      const lines = asNumber(result?.total_lines);
      const start = asNumber(result?.start_line);
      const end = asNumber(result?.end_line);
      const partial =
        start !== undefined &&
        end !== undefined &&
        (start > 1 || (lines !== undefined && end < lines));
      return {
        ...(content !== undefined ? { result: content } : {}),
        ...(lines !== undefined ? { lines } : {}),
        ...(partial ? { range: { start: start!, end: end! } } : {}),
      };
    }
    case "subagent":
      return rawResult(asString(result?.reply) ?? tool.result);
    default:
      return tool.result === undefined
        ? {}
        : {
            result:
              typeof tool.result === "string" ? tool.result : JSON.stringify(tool.result, null, 2),
          };
  }
}

export function argsText(tool: AgentToolInvocation): string {
  if (tool.argumentsText !== undefined) return tool.argumentsText;
  if (nameLabel(tool) !== undefined) return "";
  if (toolCategory(tool.name) !== "generic" && toolCategory(tool.name) !== "subagent") return "";
  return Object.keys(tool.arguments).length > 0 ? JSON.stringify(tool.arguments, null, 2) : "";
}

export function toolStatus(item: Extract<AgentItem, { type: "toolCall" }>): ToolCallStatus {
  if (item.error?.code === "denied_by_user") return "denied";
  if (item.error || item.status === "incomplete") return "err";
  if (item.status === "running") return "running";
  return "ok";
}

export function commandString(tool: AgentToolInvocation): string {
  const c = tool.arguments?.command;
  return typeof c === "string" ? c : "";
}

export function editableArgs(tool: AgentToolInvocation): Record<string, unknown> | undefined {
  const cat = toolCategory(tool.name);
  return cat === "generic" || cat === "subagent" ? tool.arguments : undefined;
}

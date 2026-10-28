import { MIMEType } from "node:util";
import { canonicalJSON, parseJSON } from "@qimu/node";
import {
  CallToolResultSchema,
  type CallToolResult,
  type ContentBlock,
} from "@modelcontextprotocol/sdk/types.js";

type JSONRecord = Record<string, unknown>;
interface ToolMedia {
  mime: string;
  source: { kind: "bytes"; bytes: string } | { kind: "uri"; uri: string };
  id?: string;
  name?: string;
  metadata?: JSONRecord;
}
interface ToolContent {
  kind: "text" | "media";
  text?: string;
  media?: ToolMedia;
  metadata?: JSONRecord;
  citations?: unknown[];
}
export interface ToolOutput {
  content?: ToolContent[];
  details?: unknown;
}

function record(value: unknown): JSONRecord {
  if (value === null || typeof value !== "object" || Array.isArray(value))
    throw new Error("computer: tool metadata must be an object");
  return value as JSONRecord;
}

function metadata(value: unknown): JSONRecord {
  const result = value === undefined ? {} : record(value);
  if (Object.keys(result).some((key) => !key))
    throw new Error("computer: tool metadata keys cannot be empty");
  return { ...result };
}

function fields(value: JSONRecord, allowed: string[]): void {
  if (Object.keys(value).some((key) => !allowed.includes(key)))
    throw new Error("computer: invalid portable tool content metadata");
}

function string(value: unknown): string {
  if (typeof value !== "string")
    throw new Error("computer: tool content string is invalid");
  return value;
}

function mime(value: string): string {
  new MIMEType(value);
  return value;
}

function absoluteURI(value: string): URL {
  if (
    !/^[A-Za-z][A-Za-z0-9+.-]*:/.test(value) ||
    /[\p{Cc}]/u.test(value) ||
    /%(?![A-Fa-f0-9]{2})/.test(value)
  )
    throw new Error("computer: tool media URI is invalid");
  const uri = new URL(value);
  if (!uri.host && !uri.pathname)
    throw new Error("computer: tool media requires an absolute target URI");
  return uri;
}

function inferredMime(declared: string | undefined, uri: string): string {
  if (declared) return mime(declared);
  let path = "";
  try {
    path = decodeURIComponent(new URL(uri).pathname).toLowerCase();
  } catch {
    return "application/octet-stream";
  }
  const extension = path.slice(path.lastIndexOf("."));
  switch (extension) {
    case ".png":
      return "image/png";
    case ".jpg":
    case ".jpeg":
      return "image/jpeg";
    case ".gif":
      return "image/gif";
    case ".webp":
      return "image/webp";
    case ".mp3":
      return "audio/mpeg";
    case ".wav":
      return "audio/wav";
    case ".pdf":
      return "application/pdf";
    case ".txt":
      return "text/plain";
    default:
      return "application/octet-stream";
  }
}

function bytes(value: string, mimeType: string): ToolMedia {
  const encoded = value.replace(/[\r\n]/g, "");
  const padding = encoded.indexOf("=");
  if (
    !encoded ||
    encoded.length % 4 !== 0 ||
    /[^A-Za-z0-9+/=]/.test(encoded) ||
    (padding !== -1 && !["=", "=="].includes(encoded.slice(padding)))
  )
    throw new Error("computer: tool media requires nonempty standard base64");
  return {
    mime: mime(mimeType),
    source: {
      kind: "bytes",
      bytes: Buffer.from(encoded, "base64").toString("base64"),
    },
  };
}

function restorePortable(
  part: ToolContent,
  content: ContentBlock,
  meta: JSONRecord,
): void {
  if (!Object.hasOwn(meta, "scope/content")) return;
  const portable = record(meta["scope/content"]);
  fields(portable, [
    "metadata",
    "citations",
    "name",
    "media_id",
    "media_metadata",
  ]);
  delete meta["scope/content"];
  const restored = metadata(portable.metadata);
  if (Object.hasOwn(restored, "mcp/content"))
    throw new Error("computer: portable metadata cannot nest an MCP envelope");
  if (Object.keys(restored).length) part.metadata = restored;
  const name = portable.name === undefined ? "" : string(portable.name);
  const id = portable.media_id === undefined ? "" : string(portable.media_id);
  const mediaMetadata = metadata(portable.media_metadata);
  if ((name || id || Object.keys(mediaMetadata).length) && !part.media)
    throw new Error("computer: text cannot carry portable media attributes");
  if (name && content.type === "resource_link")
    throw new Error("computer: portable metadata cannot replace the link name");
  if (part.media) {
    if (name) part.media.name = name;
    if (id) part.media.id = id;
    if (Object.keys(mediaMetadata).length) part.media.metadata = mediaMetadata;
  }
  if (portable.citations !== undefined) {
    if (!Array.isArray(portable.citations))
      throw new Error("computer: tool citations must be an array");
    if (portable.citations.length) {
      if (part.kind !== "text")
        throw new Error("computer: citations require text content");
      part.citations = portable.citations.map((value) => {
        const citation = record(value);
        fields(citation, ["source", "title", "quote"]);
        const source = record(citation.source);
        fields(source, ["kind", "value"]);
        const target = string(source.value);
        if (
          !target ||
          target.trim() !== target ||
          !["uri", "reference"].includes(String(source.kind))
        )
          throw new Error("computer: tool citation source is invalid");
        if (source.kind === "uri") {
          absoluteURI(target);
          if (/\p{White_Space}/u.test(target))
            throw new Error("computer: tool citation URI contains whitespace");
        }
        const title =
          citation.title === undefined ? "" : string(citation.title);
        const quote =
          citation.quote === undefined ? "" : string(citation.quote);
        if (title.trim() !== title)
          throw new Error(
            "computer: tool citation title has surrounding whitespace",
          );
        return {
          source,
          ...(title ? { title } : {}),
          ...(quote ? { quote } : {}),
        };
      });
    }
  }
}

function projectContent(content: ContentBlock): ToolContent | undefined {
  const meta = metadata(content._meta);
  if (content.type === "text" && !content.text) {
    if (Object.keys(meta).length || content.annotations)
      throw new Error("computer: empty text cannot carry metadata");
    return undefined;
  }
  const envelope: JSONRecord = { type: content.type };
  if (content.annotations) {
    const annotation = content.annotations;
    envelope.annotations = {
      ...(annotation.audience?.length ? { audience: annotation.audience } : {}),
      ...(annotation.lastModified
        ? { lastModified: annotation.lastModified }
        : {}),
      priority: annotation.priority ?? 0,
    };
  }
  let part: ToolContent;
  switch (content.type) {
    case "text":
      part = { kind: "text", text: content.text };
      break;
    case "image":
    case "audio":
      part = { kind: "media", media: bytes(content.data, content.mimeType) };
      break;
    case "resource_link": {
      absoluteURI(content.uri);
      part = {
        kind: "media",
        media: {
          mime: inferredMime(content.mimeType, content.uri),
          source: { kind: "uri", uri: content.uri },
          ...(content.name ? { name: content.name } : {}),
        },
      };
      if (content.title) envelope.title = content.title;
      if (content.description) envelope.description = content.description;
      if (content.size !== undefined) envelope.size = content.size;
      if (content.icons?.length)
        envelope.icons = content.icons.map((icon) => ({
          src: icon.src,
          ...(icon.mimeType ? { mimeType: icon.mimeType } : {}),
          ...(icon.sizes?.length ? { sizes: icon.sizes } : {}),
          ...(icon.theme ? { theme: icon.theme } : {}),
        }));
      if (!content.mimeType) envelope.mimeTypeInferred = true;
      break;
    }
    case "resource": {
      const resource = content.resource;
      const text = Object.hasOwn(resource, "text");
      if (!resource.uri || text === Object.hasOwn(resource, "blob"))
        throw new Error(
          "computer: embedded resource requires one text or blob payload",
        );
      const origin: JSONRecord = { uri: resource.uri };
      const resourceMeta = metadata(resource._meta);
      if (Object.keys(resourceMeta).length) origin._meta = resourceMeta;
      if (text) {
        const value = string(record(resource).text);
        if (!value)
          throw new Error("computer: embedded resource text cannot be empty");
        part = { kind: "text", text: value };
        if (resource.mimeType) origin.mimeType = resource.mimeType;
      } else {
        part = {
          kind: "media",
          media: bytes(
            string(record(resource).blob),
            inferredMime(resource.mimeType, resource.uri),
          ),
        };
        if (!resource.mimeType) envelope.mimeTypeInferred = true;
      }
      envelope.resource = origin;
      break;
    }
  }
  restorePortable(part, content, meta);
  if (Object.keys(meta).length) envelope._meta = meta;
  if (
    content.type !== "text" ||
    Object.keys(meta).length ||
    content.annotations
  )
    part.metadata = { ...part.metadata, "mcp/content": envelope };
  return part;
}

export function admitToolResult(
  value: unknown,
): asserts value is CallToolResult {
  CallToolResultSchema.parse(value);
  const result = record(value);
  if (!Array.isArray(result.content) || result.resultType === "input_required")
    throw new Error("computer: a complete MCP tool result is required");
  canonicalJSON(value);
}

// Receipts retain the admitted MCP result once. This is only the public Scope
// ToolOutput wire projection; parity tests execute the pinned Scope mapper.
export function projectToolOutput(
  method: string,
  result: CallToolResult,
): ToolOutput {
  admitToolResult(result);
  const detached = parseJSON(canonicalJSON(result)) as CallToolResult;
  const content = detached.content.flatMap((value) => {
    const projected = projectContent(value);
    return projected ? [projected] : [];
  });
  if (
    !content.length &&
    detached.structuredContent === undefined &&
    detached.isError
  )
    content.push({
      kind: "text",
      text: `mcp: tool ${JSON.stringify(method)} reported failure`,
    });
  return {
    ...(content.length ? { content } : {}),
    ...(Object.hasOwn(detached, "structuredContent")
      ? { details: detached.structuredContent }
      : {}),
  };
}

import { useMemo } from "react";
import { parseJSON, stringifyJSON } from "@qimu/sdk/json";
import { type Translate } from "@flame/component-sdk";
import {
  Callout,
  CodeBlock,
  Disclosure,
  Facts,
  Metadata,
  RecordBody,
  RecordGroup,
} from "@flame/ui";
import type { OperationRecord } from "./types";

export function Source({
  value,
  label,
  t,
}: {
  value: string;
  label: string;
  t: Translate;
}) {
  return (
    <CodeBlock
      label={label}
      caption={label}
      labels={{
        copy: t("source.copy"),
        copied: t("source.copied"),
        wrap: t("source.wrap"),
        unwrap: t("source.unwrap"),
      }}
    >
      {value}
    </CodeBlock>
  );
}

function object(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

export function externalURL(value: string): string | undefined {
  try {
    const url = new URL(value);
    if (
      !["https:", "http:"].includes(url.protocol) ||
      url.username ||
      url.password
    )
      return undefined;
    return url.href;
  } catch {
    return undefined;
  }
}

function mediaURL(mime: unknown, data: unknown, kind: "image" | "audio") {
  const allowed =
    kind === "image"
      ? ["image/png", "image/jpeg", "image/webp", "image/gif"]
      : [
          "audio/wav",
          "audio/x-wav",
          "audio/mpeg",
          "audio/mp3",
          "audio/ogg",
          "audio/webm",
          "audio/mp4",
          "audio/flac",
        ];
  if (
    typeof mime !== "string" ||
    !allowed.includes(mime) ||
    typeof data !== "string" ||
    data.length === 0 ||
    data.length % 4 !== 0 ||
    !/^[A-Za-z0-9+/]*={0,2}$/.test(data)
  )
    return undefined;
  return `data:${mime};base64,${data}`;
}

function Content({
  value,
  index,
  t,
}: {
  value: unknown;
  index: number;
  t: Translate;
}) {
  if (!object(value))
    return (
      <Source value={stringifyJSON(value)} label={t("result.content")} t={t} />
    );
  if (value.type === "text" && typeof value.text === "string")
    return <RecordBody>{value.text}</RecordBody>;
  if (value.type === "image") {
    const src = mediaURL(value.mimeType, value.data, "image");
    if (src)
      return (
        <figure style={{ minWidth: 0, margin: 0 }}>
          <img
            src={src}
            alt={t("result.image", { number: String(index + 1) })}
            style={{ display: "block", maxWidth: "100%", height: "auto" }}
          />
          <figcaption>
            <Metadata>{t("result.captured")}</Metadata>
          </figcaption>
        </figure>
      );
  }
  if (value.type === "audio") {
    const src = mediaURL(value.mimeType, value.data, "audio");
    if (src)
      return (
        <audio
          controls
          preload="none"
          src={src}
          aria-label={t("result.audio", { number: String(index + 1) })}
          style={{ maxWidth: "100%" }}
        />
      );
  }
  if (value.type === "resource" && object(value.resource)) {
    const resource = value.resource;
    if (typeof resource.text === "string")
      return (
        <Source
          t={t}
          value={resource.text}
          label={
            typeof resource.uri === "string"
              ? resource.uri
              : t("result.resource")
          }
        />
      );
    if (typeof resource.mimeType === "string") {
      const kind = resource.mimeType.startsWith("image/") ? "image" : "audio";
      if (mediaURL(resource.mimeType, resource.blob, kind))
        return (
          <Content
            value={{
              type: kind,
              mimeType: resource.mimeType,
              data: resource.blob,
            }}
            index={index}
            t={t}
          />
        );
    }
  }
  if (value.type === "resource_link" && typeof value.uri === "string") {
    const href = externalURL(value.uri);
    const label =
      typeof value.title === "string"
        ? value.title
        : typeof value.name === "string"
          ? value.name
          : value.uri;
    return (
      <RecordGroup>
        {href ? (
          <a href={href} target="_blank" rel="noopener noreferrer">
            {label}
          </a>
        ) : (
          <RecordBody>{label}</RecordBody>
        )}
        <Metadata>{value.uri}</Metadata>
        {typeof value.description === "string" && (
          <RecordBody>{value.description}</RecordBody>
        )}
      </RecordGroup>
    );
  }
  return (
    <Source value={stringifyJSON(value)} label={t("result.content")} t={t} />
  );
}

export function Outcome({
  record,
  t,
}: {
  record: OperationRecord;
  t: Translate;
}) {
  const outcome = useMemo(() => {
    if (record.outcomeJSON === undefined) return undefined;
    const saved = parseJSON(record.outcomeJSON);
    if (record.kind === "tool") return { result: saved };
    if (!object(saved))
      throw new Error("computer: invalid saved command outcome");
    if (object(saved.failure)) return { failure: saved.failure };
    if (object(saved.value) && object(saved.value.result))
      return { result: saved.value.result, cleanup: saved.value.cleanup };
    return { value: saved.value };
  }, [record]);
  return (
    <RecordGroup>
      <Facts
        values={[
          [t("history.identity"), record.id],
          [t("history.method"), record.method],
          [t("history.time"), record.admittedAt],
          [
            t("history.dispatch"),
            t(
              record.dispatched
                ? "history.dispatched"
                : "history.notDispatched",
            ),
          ],
          [
            t("history.outcome"),
            t(record.settled ? "history.settled" : "history.unknown"),
          ],
        ]}
      />
      {!outcome && <Callout kind="warning">{t("result.unknown")}</Callout>}
      {outcome?.failure && (
        <Callout kind="caution">
          <RecordBody>
            {String(outcome.failure.code)}: {String(outcome.failure.message)}
          </RecordBody>
        </Callout>
      )}
      {outcome?.cleanup === "unconfirmed" && (
        <Callout kind="warning">{t("result.cleanup")}</Callout>
      )}
      {outcome && object(outcome.result) && (
        <RecordGroup aria-label={t("result.content")}>
          {outcome.result.isError === true && (
            <Callout kind="caution">{t("result.toolError")}</Callout>
          )}
          {Array.isArray(outcome.result.content) &&
            outcome.result.content.map((item, index) => (
              <Content key={index} value={item} index={index} t={t} />
            ))}
          {outcome.result.structuredContent !== undefined && (
            <Disclosure title={t("result.structured")}>
              <Source
                t={t}
                value={stringifyJSON(outcome.result.structuredContent)}
                label={t("result.structured")}
              />
            </Disclosure>
          )}
        </RecordGroup>
      )}
      {outcome?.value !== undefined && (
        <Source
          t={t}
          value={stringifyJSON(outcome.value)}
          label={t("result.content")}
        />
      )}
      <Disclosure title={t("result.input")}>
        <Source value={record.inputJSON} label={t("result.input")} t={t} />
      </Disclosure>
      <Disclosure title={t("result.invocation")}>
        <Source
          value={record.invocationJSON}
          label={t("result.invocation")}
          t={t}
        />
      </Disclosure>
      {record.outcomeJSON !== undefined && (
        <Disclosure title={t("result.original")}>
          <Source
            value={record.outcomeJSON}
            label={t("result.original")}
            t={t}
          />
        </Disclosure>
      )}
    </RecordGroup>
  );
}

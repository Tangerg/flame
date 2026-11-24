import { useMemo, useState } from "react";
import { parseJSON, stringifyJSON } from "@frame/sdk/json";
import { type Translate } from "@flame/component-sdk";
import {
  Callout,
  CodeBlock,
  Disclosure,
  EvidenceImage,
  Facts,
  ImageNotice,
  Metadata,
  RecordBody,
  RecordGroup,
  imageSizeFromBase64,
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

function CapturedImage({
  src,
  data,
  label,
  t,
}: {
  src: string;
  data: string;
  label: string;
  t: Translate;
}) {
  const [failedSource, setFailedSource] = useState<string>();
  const size = useMemo(() => imageSizeFromBase64(data), [data]);
  if (failedSource === src)
    return <ImageNotice label={label} detail={t("result.imageUnavailable")} />;
  return (
    <span onErrorCapture={() => setFailedSource(src)}>
      <EvidenceImage
        src={src}
        width={size?.width}
        height={size?.height}
        decoding="async"
        alt={label}
        labels={{
          view: t("image.view"),
          close: t("image.close"),
          fit: t("image.fit"),
          zoomIn: t("image.zoomIn"),
          zoomOut: t("image.zoomOut"),
          description: t("image.description"),
        }}
      />
    </span>
  );
}

type ContentRecord = Record<string, unknown>;

function rawContent(value: unknown, t: Translate) {
  return (
    <Source value={stringifyJSON(value)} label={t("result.content")} t={t} />
  );
}

function ImageContent({
  src,
  data,
  index,
  t,
}: {
  src: string;
  data: unknown;
  index: number;
  t: Translate;
}) {
  return (
    <figure style={{ minWidth: 0, margin: 0 }}>
      <CapturedImage
        src={src}
        data={String(data)}
        label={t("result.image", { number: String(index + 1) })}
        t={t}
      />
      <figcaption>
        <Metadata>{t("result.captured")}</Metadata>
      </figcaption>
    </figure>
  );
}

function AudioContent({
  src,
  index,
  t,
}: {
  src: string;
  index: number;
  t: Translate;
}) {
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

// An embedded resource shows its text as source, or its blob as the media it
// declares; anything else is shown raw.
function EmbeddedResource({
  value,
  resource,
  index,
  t,
}: {
  value: ContentRecord;
  resource: ContentRecord;
  index: number;
  t: Translate;
}) {
  const uri =
    typeof resource.uri === "string" && resource.uri.trim()
      ? resource.uri
      : undefined;
  if (typeof resource.text === "string")
    return (
      <Source t={t} value={resource.text} label={uri ?? t("result.resource")} />
    );
  if (typeof resource.mimeType !== "string") return rawContent(value, t);
  const kind = resource.mimeType.startsWith("image/") ? "image" : "audio";
  if (!mediaURL(resource.mimeType, resource.blob, kind))
    return rawContent(value, t);
  return (
    <RecordGroup data-focus-unit="">
      {uri && <RecordBody>{uri}</RecordBody>}
      <Content
        value={{ type: kind, mimeType: resource.mimeType, data: resource.blob }}
        index={index}
        t={t}
      />
    </RecordGroup>
  );
}

function ResourceLink({
  value,
  uri,
  t,
}: {
  value: ContentRecord;
  uri: string;
  t: Translate;
}) {
  const href = externalURL(uri);
  const label =
    [value.title, value.name, uri].find(
      (candidate): candidate is string =>
        typeof candidate === "string" && candidate.trim() !== "",
    ) ?? t("result.resource");
  return (
    <RecordGroup data-prose="" data-focus-unit="">
      <RecordBody>
        {href ? (
          <a href={href} target="_blank" rel="noopener noreferrer">
            {label}
          </a>
        ) : (
          label
        )}
        {uri.trim() !== "" && label !== uri && (
          <>
            <br />
            <span data-slot="link-destination">{uri}</span>
          </>
        )}
      </RecordBody>
      {typeof value.description === "string" && (
        <RecordBody>{value.description}</RecordBody>
      )}
    </RecordGroup>
  );
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
  if (!object(value)) return rawContent(value, t);
  switch (value.type) {
    case "text":
      if (typeof value.text === "string")
        return <RecordBody>{value.text}</RecordBody>;
      break;
    case "image": {
      const src = mediaURL(value.mimeType, value.data, "image");
      if (src)
        return <ImageContent src={src} data={value.data} index={index} t={t} />;
      break;
    }
    case "audio": {
      const src = mediaURL(value.mimeType, value.data, "audio");
      if (src) return <AudioContent src={src} index={index} t={t} />;
      break;
    }
    case "resource":
      if (object(value.resource))
        return (
          <EmbeddedResource
            value={value}
            resource={value.resource}
            index={index}
            t={t}
          />
        );
      break;
    case "resource_link":
      if (typeof value.uri === "string")
        return <ResourceLink value={value} uri={value.uri} t={t} />;
      break;
  }
  return rawContent(value, t);
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

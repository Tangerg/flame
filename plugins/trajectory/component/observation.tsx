import { useCallback, useMemo, useState } from "react";
import type { ReactNode, RefObject } from "react";
import type { Translate } from "@flame/component-sdk";
import { Activity, Cpu, Terminal, FileText, Server } from "@flame/icons/react";
import type { TrajectoryEntry } from "@flame/runtime-client/wire";
import { prettyJSON } from "@frame/sdk/json";
import {
  Badge,
  CodeBlock,
  Disclosure,
  EvidenceImage,
  ImageNotice,
  Facts,
  Metadata,
  RecordBody,
  RecordMeta,
  RecordSummary,
  imageSizeFromBase64,
} from "@flame/ui";
import { evidence, readable, statusTone } from "./recordFacts";
import { observationFindings } from "./findings";

export function title(
  row: TrajectoryEntry,
  t: Translate,
  clock: Intl.DateTimeFormat,
) {
  const title =
    row.type === "run"
      ? t("title.run", { provider: row.run.provider, model: row.run.model })
      : row.type === "model"
        ? t("title.model")
        : row.type === "endpoint"
          ? t("title.endpoint", {
              provider: row.endpoint.provider,
              model: row.endpoint.model,
            })
          : row.item.type === "toolCall"
            ? row.item.tool.name
            : t(`item.${row.item.type}`);
  const tone = statusTone(row);
  const { state: label, approvalInline } = readable(row, t);
  return (
    <RecordSummary
      icon={
        row.type === "run"
          ? Activity
          : row.type === "model"
            ? Cpu
            : row.type === "endpoint"
              ? Server
              : row.item.type === "toolCall"
                ? Terminal
                : FileText
      }
      title={title}
      description={
        <RecordMeta>
          {tone === "neutral" && <span>{label}</span>}
          {row.type === "run" && <code>{row.run.id}</code>}
          <time dateTime={row.occurredAt}>
            {clock.format(new Date(row.occurredAt))}
          </time>
          {approvalInline && <span>{approvalInline}</span>}
        </RecordMeta>
      }
      status={tone !== "neutral" && <Badge tone={tone}>{label}</Badge>}
    />
  );
}

function RecordedImage({
  mime,
  data,
  t,
}: {
  mime: string;
  data: string;
  t: Translate;
}) {
  const src = `data:${mime};base64,${data}`;
  const size = useMemo(() => imageSizeFromBase64(data), [data]);
  const alt = t("image.alt", { mime });
  const [failedSource, setFailedSource] = useState<string>();
  if (failedSource === src)
    return <ImageNotice label={alt} detail={t("image.unavailable")} />;
  return (
    <span onErrorCapture={() => setFailedSource(src)}>
      <EvidenceImage
        src={src}
        width={size?.width}
        height={size?.height}
        decoding="async"
        alt={alt}
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

interface EvidenceScrollPosition {
  row: TrajectoryEntry;
  left: number;
  top: number;
}

export interface ExpandedRecord {
  evidenceOpen: boolean;
  sourcePosition: RefObject<EvidenceScrollPosition | null>;
}

// A scroll position belongs to the record it was taken from, so returning to
// the same record restores it and opening another record starts at the top.
function useEvidenceScrollRestoration(
  recorded: TrajectoryEntry,
  sourcePosition: RefObject<EvidenceScrollPosition | null> | undefined,
) {
  return useCallback(
    (node: HTMLDivElement | null) => {
      if (!node || !sourcePosition) return;
      const saved = sourcePosition.current;
      if (saved?.row === recorded)
        node.scrollTo({
          left: saved.left,
          top: saved.top,
          behavior: "instant",
        });
      return () => {
        if (node.isConnected && node.clientWidth && node.clientHeight)
          sourcePosition.current = {
            row: recorded,
            left: node.scrollLeft,
            top: node.scrollTop,
          };
      };
    },
    [recorded, sourcePosition],
  );
}

function recordedBody(row: TrajectoryEntry, t: Translate): ReactNode[] {
  if (row.type !== "item") return [];
  const item = row.item;
  if (item.type === "userMessage" || item.type === "agentMessage")
    return (item.content ?? []).flatMap((part, index) =>
      part.type === "image" ? (
        <RecordedImage key={index} mime={part.mime} data={part.data} t={t} />
      ) : part.type === "reference" ? (
        <RecordBody key={index}>{`${part.label}\n\n${part.text}`}</RecordBody>
      ) : part.text ? (
        <RecordBody key={index}>{part.text}</RecordBody>
      ) : (
        []
      ),
    );
  const text =
    item.type === "reasoning"
      ? item.text
      : item.type === "compaction"
        ? item.summary
        : undefined;
  return text ? [<RecordBody key="text">{text}</RecordBody>] : [];
}

export function Observation({
  row: recorded,
  t,
  expansion,
  onEvidenceOpenChange,
}: {
  row: TrajectoryEntry;
  t: Translate;
  expansion?: ExpandedRecord;
  onEvidenceOpenChange: (open: boolean) => void;
}) {
  const sourcePosition = expansion?.sourcePosition;
  const sourceBodyRef = useEvidenceScrollRestoration(recorded, sourcePosition);
  const row = evidence(recorded);
  const body = recordedBody(row, t);
  const { facts, notes } = observationFindings(row, t);
  const raw = (
    <CodeBlock
      label={t("evidence.recorded")}
      language="json"
      sourceBodyRef={
        sourcePosition && (!body.length || expansion?.evidenceOpen)
          ? sourceBodyRef
          : undefined
      }
      labels={{
        copy: t("code.copy"),
        wrap: t("code.wrap"),
        unwrap: t("code.unwrap"),
      }}
    >
      {prettyJSON(row)}
    </CodeBlock>
  );
  return (
    <>
      <Facts values={facts} />
      {notes.map((note, index) => (
        <Metadata key={index}>{note}</Metadata>
      ))}
      {body.length ? (
        <Disclosure
          title={t("evidence.recorded")}
          open={expansion?.evidenceOpen ?? false}
          onOpenChange={onEvidenceOpenChange}
        >
          {raw}
        </Disclosure>
      ) : (
        raw
      )}
      {body}
    </>
  );
}

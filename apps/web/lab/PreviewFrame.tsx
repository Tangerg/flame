import * as stylex from "@stylexjs/stylex";
import { previewStyles as styles } from "./previewStyles";
import { useEffect, useRef, useState } from "react";
import { messages } from "./messages";
import { ObservationLog, parseObservationMessage } from "./observations";
import { previewURL, type LabSelection } from "./selection";
import { localize, type LabStory } from "./story";

const PREVIEW_READY_TIMEOUT_MS = 30_000;

interface PreviewFrameProps {
  story: LabStory;
  selection: LabSelection;
  width: number;
  height?: number;
  generation: number;
  log: ObservationLog;
}

export function PreviewFrame(props: PreviewFrameProps) {
  const { story, selection, generation } = props;
  const identity = `${story.id}:${selection.variantId}:${selection.theme}:${selection.locale}:${selection.motion}:${selection.fontSize}:${generation}`;
  return <PreviewInstance key={identity} {...props} />;
}

function PreviewInstance({
  story,
  selection,
  width,
  height = story.height,
  log,
}: PreviewFrameProps) {
  const frame = useRef<HTMLIFrameElement>(null);
  const [status, setStatus] = useState<"loading" | "ready" | "failed">("loading");
  const [instanceId] = useState(() => `${story.id}:${crypto.randomUUID()}`);
  const src = previewURL(story, selection, instanceId);
  const t = (key: keyof typeof messages) => localize(messages[key], selection.locale);

  useEffect(() => {
    const timeout = window.setTimeout(() => setStatus("failed"), PREVIEW_READY_TIMEOUT_MS);
    function receive(event: MessageEvent<unknown>) {
      if (event.origin !== window.location.origin || event.source !== frame.current?.contentWindow)
        return;
      const message = parseObservationMessage(event.data);
      if (!message || message.instanceId !== instanceId) return;
      if (message.kind === "ready") {
        window.clearTimeout(timeout);
        setStatus("ready");
      } else if (message.kind === "error") {
        window.clearTimeout(timeout);
        setStatus("failed");
      }
      if (message.kind !== "ready") log.append(message, Date.now());
    }
    window.addEventListener("message", receive);
    return () => {
      window.clearTimeout(timeout);
      window.removeEventListener("message", receive);
    };
  }, [instanceId, log]);

  return (
    <div {...stylex.props(styles.preview)} style={{ width, height }} data-preview-status={status}>
      <iframe
        {...stylex.props(styles.iframe)}
        ref={frame}
        src={src}
        title={`${t("frameTitle")} · ${localize(story.title, selection.locale)} · ${selection.theme}`}
        width={width}
        height={height}
        onError={() => setStatus("failed")}
      />
      {status !== "ready" && (
        <div {...stylex.props(styles.previewStatus)} role="status">
          <span
            {...stylex.props(styles.statusDot, status === "failed" && styles.failedDot)}
            data-status={status}
          />
          {t(status === "loading" ? "loading" : "failed")}
        </div>
      )}
    </div>
  );
}

export function GalleryPreview(props: Parameters<typeof PreviewFrame>[0]) {
  const container = useRef<HTMLDivElement>(null);
  const [scale, setScale] = useState(1);
  const [viewport, setViewport] = useState({ width: props.width, height: props.story.height });
  useEffect(() => {
    const node = container.current;
    if (!node) return;
    const observer = new ResizeObserver(() => {
      setViewport({ width: Math.min(node.clientWidth, props.width), height: node.clientHeight });
      setScale(Math.min(node.clientWidth / props.width, node.clientHeight / props.story.height, 1));
    });
    observer.observe(node);
    return () => observer.disconnect();
  }, [props.width, props.story.height]);
  return (
    <div {...stylex.props(styles.galleryPreview)} ref={container}>
      {props.story.kind === "component" ? (
        <PreviewFrame {...props} width={viewport.width} height={viewport.height} />
      ) : (
        <div style={{ width: props.width * scale, height: props.story.height * scale }}>
          <div {...stylex.props(styles.scaledFrame)} style={{ transform: `scale(${scale})` }}>
            <PreviewFrame {...props} />
          </div>
        </div>
      )}
    </div>
  );
}

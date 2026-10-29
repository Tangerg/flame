import {
  isValidElement,
  useEffect,
  useId,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import {
  useGetFieldValue,
  type ComponentRenderProps,
} from "@openuidev/react-lang";
import type { Translate } from "@flame/component-sdk";
import {
  Button,
  CodeBlock,
  DocumentBody,
  EvidenceImage,
  ImageNotice,
  Metadata,
  RecordBody,
  RecordForm,
  ScrollRegion,
  Toolbar,
} from "@flame/ui";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { renderToString } from "katex";
import type { DisplayProps } from "../contracts/catalog";
import { isSafeURL, stateKey } from "../contracts/document";
import { useGenUIInteraction } from "./interaction";
import { useLocalValue } from "./state";

function ImageResource({
  src,
  alt,
  caption,
  inline,
  t,
}: DisplayProps["Image"] & { inline?: boolean; t: Translate }) {
  const [status, setStatus] = useState<"loading" | "ready" | "failed">(
    "loading",
  );
  const Container = inline ? "span" : "figure";
  return (
    <Container
      style={{ margin: 0, minWidth: 0 }}
      aria-busy={status === "loading"}
    >
      {status === "failed" ? (
        <>
          <ImageNotice label={alt} detail={t("image.unavailable")} />
          <Button onClick={() => setStatus("loading")}>{t("retry")}</Button>
        </>
      ) : (
        <span
          onLoadCapture={() => setStatus("ready")}
          onErrorCapture={() => setStatus("failed")}
        >
          <EvidenceImage src={src} alt={alt} />
        </span>
      )}
      {caption && <figcaption>{caption}</figcaption>}
    </Container>
  );
}

export function ImageView({
  props,
  t,
}: ComponentRenderProps<DisplayProps["Image"]> & { t: Translate }) {
  if (!isSafeURL(props.src, "image"))
    throw new Error("genui: invalid image source");
  return <ImageResource key={props.src} {...props} t={t} />;
}

function codeLabels(t: Translate) {
  return {
    copy: t("code.copy"),
    copied: t("code.copied"),
    wrap: t("code.wrap"),
    unwrap: t("code.unwrap"),
  };
}

export function CodeView({
  props,
  t,
}: ComponentRenderProps<DisplayProps["Code"]> & { t: Translate }) {
  return (
    <CodeBlock
      label={t("code.label")}
      language={props.language}
      labels={codeLabels(t)}
    >
      {props.code}
    </CodeBlock>
  );
}

export function MarkdownView({
  props,
  t,
}: ComponentRenderProps<DisplayProps["Markdown"]> & { t: Translate }) {
  return (
    <DocumentBody>
      <ReactMarkdown
        skipHtml
        remarkPlugins={[remarkGfm]}
        urlTransform={(url, key) =>
          isSafeURL(url, key === "src" ? "image" : "link") ? url : ""
        }
        components={{
          a: ({ href, children }) =>
            href ? (
              <a href={href} target="_blank" rel="noreferrer noopener">
                {children}
              </a>
            ) : (
              <span>{children}</span>
            ),
          img: ({ src, alt }) =>
            typeof src === "string" && src ? (
              <ImageResource key={src} src={src} alt={alt ?? ""} inline t={t} />
            ) : (
              <ImageNotice label={alt} detail={t("image.unavailable")} />
            ),
          pre: ({ children }) => {
            const code = isValidElement<{
              children?: ReactNode;
              className?: string;
            }>(children)
              ? children.props
              : undefined;
            const source =
              typeof code?.children === "string"
                ? code.children
                : String(children ?? "");
            const language = code?.className?.match(
              /(?:^|\s)language-([^\s]+)/,
            )?.[1];
            return (
              <CodeBlock
                label={t("code.label")}
                language={language}
                labels={codeLabels(t)}
              >
                {source}
              </CodeBlock>
            );
          },
          table: ({ children }) => (
            <ScrollRegion label={t("markdown.table")}>
              <table>
                <caption style={{ textAlign: "start" }}>
                  {t("markdown.table")}
                </caption>
                {children}
              </table>
            </ScrollRegion>
          ),
        }}
      >
        {props.text}
      </ReactMarkdown>
    </DocumentBody>
  );
}

export function MathView({
  props,
  t,
}: ComponentRenderProps<DisplayProps["Math"]> & { t: Translate }) {
  const math = useMemo(
    () =>
      renderToString(props.expression, {
        displayMode: props.display ?? false,
        output: "mathml",
        trust: false,
        throwOnError: true,
        maxExpand: 1000,
      }),
    [props.expression, props.display],
  );
  return (
    <ScrollRegion label={t("math.label")}>
      <span dangerouslySetInnerHTML={{ __html: math }} />
    </ScrollRegion>
  );
}

export function SvgView({
  props,
  renderNode,
}: ComponentRenderProps<DisplayProps["Svg"]>) {
  return (
    <svg
      role="img"
      aria-label={props.label}
      viewBox={props.viewBox}
      width={props.width ?? "100%"}
      height={props.height}
      fill="currentColor"
      style={{
        display: "block",
        maxWidth: "100%",
        height: props.height ?? "auto",
      }}
    >
      <title>{props.label}</title>
      {renderNode(props.children)}
    </svg>
  );
}

export function TimerView({
  props,
  t,
}: ComponentRenderProps<DisplayProps["Timer"]> & { t: Translate }) {
  const [remaining, setRemaining] = useLocalValue<number>(props.field);
  const [running, setRunning] = useLocalValue<boolean>(props.runningState);
  const read = useGetFieldValue();
  const interaction = useGenUIInteraction();
  const labelId = useId();
  useEffect(() => {
    if (!running) return;
    let previous = performance.now();
    const interval = setInterval(() => {
      const now = performance.now();
      const elapsed = (now - previous) / 1000;
      previous = now;
      const current = read(undefined, stateKey(props.field)) as number;
      const next = Math.max(0, current - elapsed);
      setRemaining(next);
      if (next === 0) setRunning(false);
    }, 250);
    return () => clearInterval(interval);
  }, [running, props.field, read, setRemaining, setRunning]);
  const seconds = Math.ceil(remaining);
  return (
    <RecordForm aria-label={props.label}>
      <span id={labelId}>{props.label}</span>
      <output role="timer" aria-labelledby={labelId} aria-live="off">
        {Math.floor(seconds / 60)}:{String(seconds % 60).padStart(2, "0")}
      </output>
      <Toolbar label={props.label}>
        <Button
          disabled={interaction.blocked || remaining === 0}
          onClick={() => setRunning(!running)}
        >
          {t(running ? "timer.pause" : "timer.start")}
        </Button>
        <Button
          disabled={interaction.blocked}
          onClick={() => interaction.reset([props.field, props.runningState])}
        >
          {t("reset")}
        </Button>
      </Toolbar>
    </RecordForm>
  );
}

export function FlashcardView({
  props,
  t,
}: ComponentRenderProps<DisplayProps["Flashcard"]> & { t: Translate }) {
  const [flipped, flip] = useLocalValue<boolean>(props.field);
  return (
    <RecordForm aria-label={t("flashcard.label")}>
      <Metadata>
        {t(flipped ? "flashcard.answer" : "flashcard.question")}
      </Metadata>
      <RecordBody>{flipped ? props.back : props.front}</RecordBody>
      <Button onClick={() => flip(!flipped)}>
        {t(flipped ? "flashcard.showQuestion" : "flashcard.showAnswer")}
      </Button>
    </RecordForm>
  );
}

export function YouTubeView({
  props,
}: ComponentRenderProps<DisplayProps["YouTube"]>) {
  const start = props.start ? `?start=${props.start}` : "";
  return (
    <figure style={{ margin: 0, minWidth: 0 }}>
      <iframe
        title={props.title}
        src={`https://www.youtube-nocookie.com/embed/${props.videoId}${start}`}
        loading="lazy"
        allow="fullscreen; picture-in-picture"
        allowFullScreen
        referrerPolicy="strict-origin-when-cross-origin"
        style={{
          display: "block",
          width: "100%",
          aspectRatio: "16 / 9",
          border: 0,
        }}
      />
      <figcaption>
        <a
          href={`https://www.youtube.com/watch?v=${props.videoId}`}
          target="_blank"
          rel="noreferrer noopener"
        >
          {props.title}
        </a>
      </figcaption>
    </figure>
  );
}

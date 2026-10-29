import { Children, isValidElement, useMemo, type ReactNode } from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import {
  createTranslator,
  type ComponentContext,
  type Translate,
} from "@flame/component-sdk";
import {
  Callout,
  CodeBlock,
  DocumentBody,
  ImageNotice,
  Panel,
  ScrollRegion,
  TaskListItem,
  remarkCallouts,
  taskListContent,
  visibleText,
  type CalloutKind,
} from "@flame/ui";
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { de } from "./locales/de";

const CATALOGS = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };

function blockChildren(children: ReactNode): ReactNode[] {
  return Children.toArray(children).filter(
    (child) => typeof child !== "string" || child.trim() !== "",
  );
}

const remarkPlugins = [remarkGfm, remarkCallouts];

// Destinations stay visible as text; no authored URL is assigned to a DOM attribute.
function markdownComponents(t: Translate): Components {
  return {
    a({ children, href, title }) {
      return (
        <span title={title}>
          {children}
          {href && visibleText(children) !== href && (
            <small data-slot="link-destination"> ({href})</small>
          )}
        </span>
      );
    },
    img({ src, alt, title }) {
      const source = typeof src === "string" ? src : "";
      return (
        <ImageNotice
          label={alt || undefined}
          title={title}
          detail={/^data:/i.test(source) ? t("image.embedded") : source}
        />
      );
    },
    pre({ children, node }) {
      const child = Children.toArray(children)[0];
      if (
        !isValidElement<{
          children?: ReactNode;
          className?: string;
          node?: { data?: { meta?: string } };
        }>(child)
      )
        return children;
      const language = child.props.className?.match(/language-([^\s]+)/)?.[1];
      const caption = [language, child.props.node?.data?.meta]
        .filter(Boolean)
        .join(" ");
      return (
        <CodeBlock
          label={t("code.block", { n: node?.position?.start.line ?? 1 })}
          language={language}
          caption={caption || undefined}
        >
          {String(child.props.children ?? "").replace(/\n$/, "")}
        </CodeBlock>
      );
    },
    code({ children }) {
      return <code dir="ltr">{children}</code>;
    },
    li({ children, className }) {
      const task = className?.includes("task-list-item")
        ? taskListContent(children)
        : null;
      return task ? (
        <TaskListItem checked={task.checked} label={task.label}>
          {task.children}
        </TaskListItem>
      ) : (
        <li>{children}</li>
      );
    },
    ol({ children, start, className }) {
      return (
        <ol dir="auto" start={start} className={className}>
          {blockChildren(children)}
        </ol>
      );
    },
    ul({ children, className }) {
      return (
        <ul dir="auto" className={className}>
          {blockChildren(children)}
        </ul>
      );
    },
    blockquote({ children, node }) {
      const kind = node?.properties.dataCallout as CalloutKind | undefined;
      return kind ? (
        <Callout kind={kind}>{blockChildren(children)}</Callout>
      ) : (
        <blockquote dir="auto">{children}</blockquote>
      );
    },
    table({ children, node }) {
      return (
        <ScrollRegion
          label={t("table.label", { n: node?.position?.start.line ?? 1 })}
        >
          <table dir="auto">{children}</table>
        </ScrollRegion>
      );
    },
    th({ children, style }) {
      return (
        <th dir="auto" style={style}>
          {children}
        </th>
      );
    },
    td({ children, style }) {
      return (
        <td dir="auto" style={style}>
          {children}
        </td>
      );
    },
    p({ children }) {
      return <p dir="auto">{children}</p>;
    },
    h1({ children }) {
      return <h1 dir="auto">{children}</h1>;
    },
    h2({ children }) {
      return <h2 dir="auto">{children}</h2>;
    },
    h3({ children }) {
      return <h3 dir="auto">{children}</h3>;
    },
    h4({ children }) {
      return <h4 dir="auto">{children}</h4>;
    },
    h5({ children }) {
      return <h5 dir="auto">{children}</h5>;
    },
    h6({ children }) {
      return <h6 dir="auto">{children}</h6>;
    },
  };
}

export function Markdown({ context }: { context: ComponentContext }) {
  const file = context.file;
  if (!file) throw new Error("Markdown requires a captured file context");
  const t = useMemo(
    () => createTranslator(CATALOGS, context.locale),
    [context.locale],
  );
  const components = useMemo(() => markdownComponents(t), [t]);
  return (
    <Panel label={t("panel.label")}>
      <DocumentBody>
        <ReactMarkdown
          remarkPlugins={remarkPlugins}
          components={components}
          urlTransform={(url) => url}
        >
          {file.content}
        </ReactMarkdown>
      </DocumentBody>
    </Panel>
  );
}

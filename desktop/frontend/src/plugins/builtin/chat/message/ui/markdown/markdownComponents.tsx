import type { Components } from "react-markdown";
import { Callout, TaskListItem, type CalloutKind } from "@flame/ui";
import {
  Children,
  cloneElement,
  isValidElement,
  type ComponentProps,
  type ReactElement,
  type ReactNode,
} from "react";
import { TextLink, ShikiCodeBlock } from "@/ui";
import { cn } from "@/lib/classNames";
import { FileRefLink } from "@/plugins/builtin/chat/file-references/public/FileRefLink";
import { MarkdownImage } from "./MarkdownImage";
import { MarkdownMath } from "./MarkdownMath";
import { MermaidBlock } from "./MermaidBlock";
import { MarkdownTable } from "./MarkdownTable";
import { SvgArtifact } from "./SvgArtifact";

function visibleText(children: ReactNode): string {
  return Children.toArray(children)
    .map((child) => {
      if (typeof child === "string" || typeof child === "number") return String(child);
      return isValidElement<{ children?: ReactNode }>(child)
        ? visibleText(child.props.children)
        : "";
    })
    .join("");
}

const WHITESPACE_ONLY = /^\s*$/;
const NUMERIC_CELL = /^[+-]?\d[\d,\u202f\u00a0]*(?:\.\d+)?\s*(?:%|[a-zA-Z]{1,3})?$/;
const NUMERIC_CLASS = "md-table-cell-numeric";

type MarkdownImageElementProps = ComponentProps<typeof MarkdownImage> & {
  node?: { tagName?: string };
};

function imageOnlyParagraph(children: ReactNode): ReactElement<MarkdownImageElementProps>[] | null {
  const material = Children.toArray(children).filter(
    (child) =>
      !(typeof child === "string" && WHITESPACE_ONLY.test(child)) &&
      !(isValidElement(child) && child.type === "br"),
  );
  if (
    material.length === 0 ||
    !material.every(
      (child): child is ReactElement<MarkdownImageElementProps> =>
        isValidElement<MarkdownImageElementProps>(child) && child.props.node?.tagName === "img",
    )
  )
    return null;
  return material;
}

function imagesDeferToTheLink(children: ReactNode): ReactNode {
  return Children.map(children, (child) => {
    if (!isValidElement<MarkdownImageElementProps & { children?: ReactNode }>(child)) return child;
    if (child.props.node?.tagName === "img") return cloneElement(child, { linked: true });
    const inner = child.props.children;
    return inner === undefined
      ? child
      : cloneElement(child, { children: imagesDeferToTheLink(inner) });
  });
}

type MarkdownElementProps = {
  children?: ReactNode;
  node?: { tagName?: string };
  checked?: boolean;
  type?: string;
};

const BLOCK_TAGS = new Set([
  "p",
  "ul",
  "ol",
  "blockquote",
  "pre",
  "table",
  "h1",
  "h2",
  "h3",
  "h4",
  "h5",
  "h6",
  "hr",
]);

function taskListContent(children: ReactNode) {
  const items = Children.toArray(children).filter(
    (child) => !(typeof child === "string" && WHITESPACE_ONLY.test(child)),
  );
  const lead = items[0];
  const paragraph =
    isValidElement<MarkdownElementProps>(lead) && lead.props.node?.tagName === "p" ? lead : null;
  const [checkbox, ...content] = paragraph ? Children.toArray(paragraph.props.children) : items;
  if (
    !isValidElement<MarkdownElementProps>(checkbox) ||
    (checkbox.type !== "input" && checkbox.props.node?.tagName !== "input") ||
    checkbox.props.type !== "checkbox"
  ) {
    return null;
  }

  const blockIndex = content.findIndex(
    (child) =>
      isValidElement<MarkdownElementProps>(child) &&
      BLOCK_TAGS.has(child.props.node?.tagName ?? String(child.type)),
  );
  return {
    checked: checkbox.props.checked === true,
    label: visibleText(paragraph || blockIndex < 0 ? content : content.slice(0, blockIndex)).trim(),
    children: paragraph
      ? [cloneElement(paragraph, { children: content }), ...items.slice(1)]
      : content,
  };
}

const sharedMarkdownComponents: Components = {
  span({ className, node: _node, ...props }) {
    return className?.split(/\s+/).includes("katex-display") ? (
      <MarkdownMath className={className} {...props} />
    ) : (
      <span className={className} {...props} />
    );
  },
  p({ children, className }) {
    const images = imageOnlyParagraph(children);
    if (images && images.length > 1) {
      const galleryImages = images.map((image, index) =>
        cloneElement(image, { key: `${image.props.src ?? index}-${index}`, layout: "paragraph" }),
      );
      return (
        <p className="md-media-paragraph md-media-grid" data-markdown-image-grid="true">
          {galleryImages}
        </p>
      );
    }
    if (images?.length === 1) {
      const image = images[0]!;
      const paragraphImage = cloneElement(image, {
        key: image.props.src ?? 0,
        layout: "paragraph",
      });
      return (
        <p className="md-media-paragraph md-media-wide-block" data-wide-markdown-block="true">
          {paragraphImage}
        </p>
      );
    }
    return (
      <p className={className} dir="auto">
        {children}
      </p>
    );
  },
  h1({ children, node: _node, ...props }) {
    return (
      <h3 {...props} dir="auto" data-prose-level="1">
        {children}
      </h3>
    );
  },
  h2({ children, node: _node, ...props }) {
    return (
      <h3 {...props} dir="auto" data-prose-level="2">
        {children}
      </h3>
    );
  },
  h3({ children, node: _node, ...props }) {
    return (
      <h4 {...props} dir="auto" data-prose-level="3">
        {children}
      </h4>
    );
  },
  h4({ children, node: _node, ...props }) {
    return (
      <h5 {...props} dir="auto" data-prose-level="4">
        {children}
      </h5>
    );
  },
  h5({ children, node: _node, ...props }) {
    return (
      <h6 {...props} dir="auto" data-prose-level="5">
        {children}
      </h6>
    );
  },
  h6({ children, node: _node, ...props }) {
    return (
      <h6 {...props} dir="auto" data-prose-level="6">
        {children}
      </h6>
    );
  },
  ul({ children, className }) {
    return (
      <ul className={className} dir="auto">
        {children}
      </ul>
    );
  },
  ol({ children, className, start }) {
    return (
      <ol className={className} dir="auto" start={start}>
        {children}
      </ol>
    );
  },
  li({ children, className, node: _node, ...rest }) {
    const task = className?.includes("task-list-item") ? taskListContent(children) : null;
    if (task) {
      return (
        <TaskListItem {...rest} checked={task.checked} label={task.label}>
          {task.children}
        </TaskListItem>
      );
    }
    return (
      <li className={className} {...rest}>
        {children}
      </li>
    );
  },
  blockquote({ children, node: _node, ...props }) {
    const kind = (props as { "data-callout"?: CalloutKind })["data-callout"];
    if (kind) return <Callout kind={kind}>{children}</Callout>;
    return <blockquote dir="auto">{children}</blockquote>;
  },
  pre({ children }) {
    const child = Children.toArray(children)[0];
    if (
      Children.count(children) === 1 &&
      isValidElement<{
        children?: ReactNode;
        className?: string;
        node?: { tagName?: string };
      }>(child) &&
      child.props.node?.tagName === "code" &&
      !child.props.className
    ) {
      const code = String(child.props.children ?? "").replace(/\n$/, "");
      return <ShikiCodeBlock lang="text" code={code} />;
    }
    return <>{children}</>;
  },
  code({ className, children }) {
    const cls = String(className ?? "");
    const match = /language-([\w+-]+)/.exec(cls);
    if (!match) {
      return (
        <code className={cls} dir="ltr" data-slot="inline-code">
          {children}
        </code>
      );
    }
    const lang = match[1]!.toLowerCase();
    const codeStr = String(children ?? "").replace(/\n$/, "");
    if (lang === "mermaid") return <MermaidBlock code={codeStr} />;
    if (
      lang === "svg" ||
      ((lang === "xml" || lang === "html" || lang === "htm") &&
        /^\s*(?:<\?xml[^>]*>\s*)?<svg[\s>]/i.test(codeStr))
    )
      return <SvgArtifact code={codeStr} lang={lang} />;
    return <ShikiCodeBlock lang={lang} code={codeStr} />;
  },
  td({ children, className, align, colSpan, rowSpan, style }) {
    return (
      <td
        className={cn(className, NUMERIC_CELL.test(visibleText(children).trim()) && NUMERIC_CLASS)}
        align={align}
        colSpan={colSpan}
        rowSpan={rowSpan}
        style={style}
        dir="auto"
      >
        {children}
      </td>
    );
  },
  th({ children, className, align, colSpan, rowSpan, style }) {
    return (
      <th
        className={className}
        align={align}
        colSpan={colSpan}
        rowSpan={rowSpan}
        style={style}
        dir="auto"
      >
        {children}
      </th>
    );
  },
  img({ src, alt, title, ...rest }) {
    const decided = rest as Pick<MarkdownImageElementProps, "layout" | "linked">;
    return (
      <MarkdownImage
        src={src}
        alt={alt}
        title={title}
        layout={decided.layout}
        linked={decided.linked}
      />
    );
  },
  a({ href, title, children, node: _node, ...rest }) {
    const r = rest as {
      "data-file-ref"?: string;
      "data-file-line"?: string;
      "data-file-column"?: string;
    };
    if (r["data-file-ref"]) {
      return (
        <FileRefLink
          path={r["data-file-ref"]}
          line={Number(r["data-file-line"]) || 0}
          column={Number(r["data-file-column"]) || 0}
        />
      );
    }
    return (
      <TextLink
        {...rest}
        href={href}
        title={title}
        data-markdown-copy={Object.hasOwn(rest, "data-footnote-backref") ? "exclude" : undefined}
      >
        {imagesDeferToTheLink(children)}
      </TextLink>
    );
  },
};

export function createMarkdownComponents(markdownSource: string): Components {
  return {
    ...sharedMarkdownComponents,
    table({ children, node }) {
      const start = node?.position?.start.offset;
      const end = node?.position?.end.offset;
      const tableSource =
        typeof start === "number" && typeof end === "number"
          ? markdownSource.slice(start, end)
          : markdownSource;
      return <MarkdownTable markdownSource={tableSource}>{children}</MarkdownTable>;
    },
  };
}

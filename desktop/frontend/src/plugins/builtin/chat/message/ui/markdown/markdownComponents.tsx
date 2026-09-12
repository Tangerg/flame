import type { Components } from "react-markdown";
import {
  Children,
  cloneElement,
  isValidElement,
  type ComponentProps,
  type ReactElement,
  type ReactNode,
} from "react";
import { ExternalLink, ShikiCodeBlock } from "@/ui";
import { cn } from "@/lib/classNames";
import { FileRefLink } from "@/plugins/builtin/chat/file-references/public/FileRefLink";
import { MarkdownImage } from "./MarkdownImage";
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
/**
 * Writing with no spaces between words, which is why two paragraphs of it sit closer.
 *
 * A run of Japanese or Chinese is a solid block of even ink — no word gaps, no ascender rhythm
 * — so the same paragraph margin that separates two ragged Latin blocks reads as a hole between
 * two of these. `markdown.css` closes it, and this decides which paragraphs are in the run.
 *
 * It was `\p{Script=Han}` — kanji and hanzi only. A Japanese paragraph that happens to be all
 * kana is still Japanese, and the CSS pairs ADJACENT paragraphs, so one of them in the middle of
 * a reply reopened the gap above AND below it while the rest stayed closed: measured on
 * `それではつづきをおねがいします。` and `コンパイルエラーガアリマス。`, both unmarked beside
 * marked neighbours.
 *
 * Hangul is deliberately NOT here. Korean puts spaces between words, so it has the ragged
 * rhythm this rule exists to leave alone — and a Korean reply is uniformly unmarked rather than
 * inconsistent with itself.
 */
const UNSPACED_SCRIPT = /[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}]/u;

/**
 * A table cell that is a NUMBER, so a column of them lines up and cannot wrap.
 *
 * `markdown.css` gives the class tabular figures, a floor width and `nowrap`; what decides
 * which cells get it used to be `/^\d+$/`, which is a whole unsigned integer and nothing else.
 * The table an agent actually writes is a benchmark or a diff summary — `91.2%`, `4.2s`,
 * `1,234`, `-3` — and not one of those matched, so the digits in a column sat on proportional
 * widths and did not align.
 *
 * Deliberately a little generous at the tail: a trailing unit of up to three letters (`ms`,
 * `GB`, `px`) still reads as a quantity. Being wrong that way costs a minimum width on a short
 * cell; being wrong the other way is a column that does not line up. The expression is anchored
 * at both ends, so a sentence can never reach it.
 */
// Grouping is a comma or one of the two spaces a locale uses for it.
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

/**
 * Recursive rather than a check on the link's direct children, because markdown puts emphasis
 * between them freely — `[**![badge](x)**](url)` is the same shape as `[![badge](x)](url)` and
 * has the same answer.
 */
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
  "aria-label"?: string;
};

function codexTaskListChildren(children: ReactNode): ReactNode {
  const items = Children.toArray(children).filter(
    (child) => !(typeof child === "string" && WHITESPACE_ONLY.test(child)),
  );
  const lead = items[0];
  if (!isValidElement<MarkdownElementProps>(lead) || lead.props.node?.tagName !== "p") {
    return children;
  }

  const [checkbox, ...paragraphChildren] = Children.toArray(lead.props.children);
  if (
    !isValidElement<MarkdownElementProps>(checkbox) ||
    (checkbox.type !== "input" && checkbox.props.node?.tagName !== "input")
  ) {
    return children;
  }

  const label = visibleText(paragraphChildren).trim();
  return [
    cloneElement(checkbox, { "aria-label": label }),
    cloneElement(lead, { children: paragraphChildren }),
    ...items.slice(1),
  ];
}

const sharedMarkdownComponents: Components = {
  p({ children }) {
    const images = imageOnlyParagraph(children);
    if (images && images.length > 1) {
      const galleryImages = images.map((image, index) =>
        cloneElement(image, { key: `${image.props.src ?? index}-${index}`, allowWide: true }),
      );
      return (
        <p className="md-media-paragraph md-media-grid" data-markdown-image-grid="true">
          {galleryImages}
        </p>
      );
    }
    if (images?.length === 1) {
      const image = images[0]!;
      const wideImage = cloneElement(image, { key: image.props.src ?? 0, allowWide: true });
      return (
        <p className="md-media-paragraph md-media-wide-block" data-wide-markdown-block="true">
          {wideImage}
        </p>
      );
    }
    return (
      <p
        data-markdown-unspaced={UNSPACED_SCRIPT.test(visibleText(children)) ? "true" : undefined}
        dir="auto"
      >
        {children}
      </p>
    );
  },
  // A body opens at h3 — one below the turn heading that contains it — so a model writing
  // `# Title` cannot outrank its own turn, and `#`/`##` share a rung because a message is cut
  // by `splitStreamingBlocks` and each block renders through its OWN `ReactMarkdown`: nothing
  // here can see which levels the rest of the message used. `data-md-level` carries the authored
  // level for the type scale.
  h1({ children }) {
    return (
      <h3 dir="auto" data-md-level="1">
        {children}
      </h3>
    );
  },
  h2({ children }) {
    return (
      <h3 dir="auto" data-md-level="2">
        {children}
      </h3>
    );
  },
  h3({ children }) {
    return (
      <h4 dir="auto" data-md-level="3">
        {children}
      </h4>
    );
  },
  h4({ children }) {
    return (
      <h5 dir="auto" data-md-level="4">
        {children}
      </h5>
    );
  },
  h5({ children }) {
    return (
      <h6 dir="auto" data-md-level="5">
        {children}
      </h6>
    );
  },
  h6({ children }) {
    return (
      <h6 dir="auto" data-md-level="6">
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
    const taskListItem = className?.includes("task-list-item") ?? false;
    return (
      <li className={className} {...rest}>
        {taskListItem ? codexTaskListChildren(children) : children}
      </li>
    );
  },
  blockquote({ children }) {
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
        <code className={cls} dir="ltr">
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
  // Every prop a PARENT decided has to be named here: this is a forwarding wrapper, so anything
  // `cloneElement` set upstream — `allowWide` from an image-only paragraph, `linked` from a link
  // — is dropped unless it is read back out of `rest`.
  img({ src, alt, title, ...rest }) {
    const decided = rest as { allowWide?: boolean; linked?: boolean };
    return (
      <MarkdownImage
        src={src}
        alt={alt}
        title={title}
        allowWide={decided.allowWide}
        linked={decided.linked}
      />
    );
  },
  a({ href, title, children, ...rest }) {
    const r = rest as { "data-file-ref"?: string; "data-file-line"?: string };
    if (r["data-file-ref"]) {
      return <FileRefLink path={r["data-file-ref"]} line={Number(r["data-file-line"]) || 0} />;
    }
    return (
      <ExternalLink href={href} title={title}>
        {imagesDeferToTheLink(children)}
      </ExternalLink>
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

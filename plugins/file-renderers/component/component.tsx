import {
  Children,
  createElement,
  isValidElement,
  useMemo,
  type ReactNode,
} from "react";
import { Lexer, type Token, type Tokens } from "marked";
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
  readCalloutMarker,
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

function visibleText(children: ReactNode): string {
  return Children.toArray(children)
    .map((child) => {
      if (typeof child === "string" || typeof child === "number")
        return String(child);
      return isValidElement<{ children?: ReactNode }>(child)
        ? visibleText(child.props.children)
        : "";
    })
    .join("");
}

function calloutOf(
  tokens: Token[],
): { kind: CalloutKind; tokens: Token[] } | null {
  const [lead, ...rest] = tokens;
  if (lead?.type !== "paragraph") return null;
  const authored = readCalloutMarker(lead.raw);
  if (!authored) return null;
  const [first, ...inline] = (lead as Tokens.Paragraph).tokens;
  if (first?.type !== "text") return null;
  const marker = readCalloutMarker((first as Tokens.Text).text);
  if (marker?.kind !== authored.kind) return null;
  const body = marker.rest
    ? [
        { ...first, raw: marker.rest, text: marker.rest } as Tokens.Text,
        ...inline,
      ]
    : inline[0]?.type === "br"
      ? inline.slice(1)
      : inline;
  return {
    kind: marker.kind,
    tokens:
      body.length > 0
        ? [{ ...lead, tokens: body } as Tokens.Paragraph, ...rest]
        : rest,
  };
}

// Parsed file content never supplies HTML or executable URL attributes.
function renderTokens(tokens: Token[], t: Translate): ReactNode[] {
  return tokens.map((token, key) => {
    switch (token.type) {
      case "space":
      case "def":
      case "checkbox":
        return null;
      case "hr":
        return <hr key={key} />;
      case "br":
        return <br key={key} />;
      case "heading": {
        const value = token as Tokens.Heading;
        return createElement(
          ["h1", "h2", "h3", "h4", "h5", "h6"][value.depth - 1] ?? "h6",
          { key, dir: "auto" },
          renderTokens(value.tokens, t),
        );
      }
      case "paragraph":
        return (
          <p key={key} dir="auto">
            {renderTokens((token as Tokens.Paragraph).tokens, t)}
          </p>
        );
      case "strong":
        return (
          <strong key={key}>
            {renderTokens((token as Tokens.Strong).tokens, t)}
          </strong>
        );
      case "em":
        return (
          <em key={key}>{renderTokens((token as Tokens.Em).tokens, t)}</em>
        );
      case "del":
        return (
          <del key={key}>{renderTokens((token as Tokens.Del).tokens, t)}</del>
        );
      case "blockquote": {
        const quoted = (token as Tokens.Blockquote).tokens;
        const callout = calloutOf(quoted);
        return callout ? (
          <Callout key={key} kind={callout.kind}>
            {renderTokens(callout.tokens, t)}
          </Callout>
        ) : (
          <blockquote key={key} dir="auto">
            {renderTokens(quoted, t)}
          </blockquote>
        );
      }
      case "codespan":
        return (
          <code key={key} dir="ltr">
            {(token as Tokens.Codespan).text}
          </code>
        );
      case "code": {
        const value = token as Tokens.Code;
        return (
          <CodeBlock
            key={key}
            label={t("code.block", { n: key + 1 })}
            language={value.lang || undefined}
          >
            {value.text}
          </CodeBlock>
        );
      }
      case "link": {
        const value = token as Tokens.Link;
        const label = renderTokens(value.tokens, t);
        return (
          <span key={key} title={value.title ?? undefined}>
            {label}
            {value.href && visibleText(label) !== value.href && (
              <small data-slot="link-destination"> ({value.href})</small>
            )}
          </span>
        );
      }
      case "image": {
        const value = token as Tokens.Image;
        return (
          <ImageNotice
            key={key}
            label={value.text || undefined}
            detail={
              /^data:/i.test(value.href) ? t("image.embedded") : value.href
            }
            title={value.title ?? undefined}
          />
        );
      }
      case "list": {
        const value = token as Tokens.List;
        const rows = value.items.map((item, index) => {
          const content = renderTokens(item.tokens, t);
          if (!item.task) return <li key={index}>{content}</li>;
          const primary = item.tokens.find(
            (part) => part.type === "paragraph" || part.type === "text",
          );
          return (
            <TaskListItem
              key={index}
              checked={item.checked === true}
              label={visibleText(
                primary ? renderTokens([primary], t) : null,
              ).trim()}
            >
              {content}
            </TaskListItem>
          );
        });
        const className = value.items.some((item) => item.task)
          ? "contains-task-list"
          : undefined;
        return value.ordered ? (
          <ol
            key={key}
            className={className}
            dir="auto"
            start={typeof value.start === "number" ? value.start : undefined}
          >
            {rows}
          </ol>
        ) : (
          <ul key={key} className={className} dir="auto">
            {rows}
          </ul>
        );
      }
      case "table": {
        const value = token as Tokens.Table;
        return (
          <ScrollRegion key={key} label={t("table.label", { n: key + 1 })}>
            <table dir="auto">
              <thead>
                <tr>
                  {value.header.map((cell, index) => (
                    <th
                      key={index}
                      dir="auto"
                      style={{ textAlign: value.align[index] ?? undefined }}
                    >
                      {renderTokens(cell.tokens, t)}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {value.rows.map((row, index) => (
                  <tr key={index}>
                    {row.map((cell, column) => (
                      <td
                        key={column}
                        dir="auto"
                        style={{ textAlign: value.align[column] ?? undefined }}
                      >
                        {renderTokens(cell.tokens, t)}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </ScrollRegion>
        );
      }
      case "text": {
        const value = token as Tokens.Text;
        return value.tokens ? (
          <span key={key}>{renderTokens(value.tokens, t)}</span>
        ) : (
          value.text
        );
      }
      case "html":
      case "escape":
        return token.text;
      default:
        return token.raw;
    }
  });
}

export function Markdown({ context }: { context: ComponentContext }) {
  const file = context.file;
  if (!file) throw new Error("Markdown requires a captured file context");
  const t = useMemo(
    () => createTranslator(CATALOGS, context.locale),
    [context.locale],
  );
  const content = useMemo(
    () => renderTokens(Lexer.lex(file.content, { gfm: true }), t),
    [file.content, t],
  );
  return (
    <Panel label={t("panel.label")}>
      <DocumentBody>{content}</DocumentBody>
    </Panel>
  );
}

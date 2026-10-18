import {
  Children,
  createElement,
  isValidElement,
  useMemo,
  type ReactNode,
} from "react";
import { Lexer, type Token, type Tokens } from "marked";
import type { ComponentContext } from "@flame/plugin-sdk";
import {
  Callout,
  CodeBlock,
  DocumentBody,
  ImageNotice,
  Panel,
  RecordBody,
  ScrollRegion,
  TaskListItem,
  readCalloutMarker,
  type CalloutKind,
} from "@flame/ui";

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
function renderTokens(tokens: Token[]): ReactNode[] {
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
          renderTokens(value.tokens),
        );
      }
      case "paragraph":
        return (
          <p key={key} dir="auto">
            {renderTokens((token as Tokens.Paragraph).tokens)}
          </p>
        );
      case "strong":
        return (
          <strong key={key}>
            {renderTokens((token as Tokens.Strong).tokens)}
          </strong>
        );
      case "em":
        return <em key={key}>{renderTokens((token as Tokens.Em).tokens)}</em>;
      case "del":
        return (
          <del key={key}>{renderTokens((token as Tokens.Del).tokens)}</del>
        );
      case "blockquote": {
        const quoted = (token as Tokens.Blockquote).tokens;
        const callout = calloutOf(quoted);
        return callout ? (
          <Callout key={key} kind={callout.kind}>
            {renderTokens(callout.tokens)}
          </Callout>
        ) : (
          <blockquote key={key} dir="auto">
            {renderTokens(quoted)}
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
            label={`Code block ${key + 1}`}
            language={value.lang || undefined}
          >
            {value.text}
          </CodeBlock>
        );
      }
      case "link": {
        const value = token as Tokens.Link;
        const label = renderTokens(value.tokens);
        return (
          <span key={key} title={value.title ?? undefined}>
            {label}
            {value.href && visibleText(label) !== value.href && (
              <small> ({value.href})</small>
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
              /^data:/i.test(value.href) ? "Embedded image data" : value.href
            }
            title={value.title ?? undefined}
          />
        );
      }
      case "list": {
        const value = token as Tokens.List;
        const rows = value.items.map((item, index) => {
          const content = renderTokens(item.tokens);
          if (!item.task) return <li key={index}>{content}</li>;
          const primary = item.tokens.find(
            (part) => part.type === "paragraph" || part.type === "text",
          );
          return (
            <TaskListItem
              key={index}
              checked={item.checked === true}
              label={visibleText(
                primary ? renderTokens([primary]) : null,
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
          <ScrollRegion key={key} label={`Markdown table ${key + 1}`}>
            <table dir="auto">
              <thead>
                <tr>
                  {value.header.map((cell, index) => (
                    <th
                      key={index}
                      dir="auto"
                      style={{ textAlign: value.align[index] ?? undefined }}
                    >
                      {renderTokens(cell.tokens)}
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
                        {renderTokens(cell.tokens)}
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
          <span key={key}>{renderTokens(value.tokens)}</span>
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
  const content = useMemo(
    () => renderTokens(Lexer.lex(file.content, { gfm: true })),
    [file.content],
  );
  if (file.startLine !== 1 || file.truncated)
    return (
      <RecordBody>
        The complete file is required. Use the source view.
      </RecordBody>
    );
  return (
    <Panel label="Markdown file">
      <DocumentBody>{content}</DocumentBody>
    </Panel>
  );
}

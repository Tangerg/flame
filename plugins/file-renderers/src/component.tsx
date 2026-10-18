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
  CodeBlock,
  DocumentBody,
  Metadata,
  Panel,
  RecordBody,
  ScrollRegion,
  TaskListItem,
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
      case "blockquote":
        return (
          <blockquote key={key} dir="auto">
            {renderTokens((token as Tokens.Blockquote).tokens)}
          </blockquote>
        );
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
          <span key={key}>
            [Image{value.text ? `: ${value.text}` : ""}]
            <small>{` (${/^data:/i.test(value.href) ? "embedded image data" : value.href})`}</small>
          </span>
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
      <Metadata>
        Links and images are shown as text. Embedded data is available in
        Source.
      </Metadata>
    </Panel>
  );
}

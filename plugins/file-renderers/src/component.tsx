import { createElement, useMemo, type ReactNode } from "react";
import { Lexer, type Token, type Tokens } from "marked";
import type { ComponentContext } from "@flame/plugin-sdk";
import {
  CodeBlock,
  DocumentBody,
  Metadata,
  Panel,
  RecordBody,
  ScrollRegion,
} from "@flame/ui";

// Parsed file content never supplies HTML or executable URL attributes.
function renderTokens(tokens: Token[]): ReactNode[] {
  return tokens.map((token, key) => {
    switch (token.type) {
      case "space":
      case "def":
        return null;
      case "hr":
        return <hr key={key} />;
      case "br":
        return <br key={key} />;
      case "heading": {
        const value = token as Tokens.Heading;
        return createElement(
          ["h1", "h2", "h3", "h4", "h5", "h6"][value.depth - 1] ?? "h6",
          { key },
          renderTokens(value.tokens),
        );
      }
      case "paragraph":
        return (
          <p key={key}>{renderTokens((token as Tokens.Paragraph).tokens)}</p>
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
          <blockquote key={key}>
            {renderTokens((token as Tokens.Blockquote).tokens)}
          </blockquote>
        );
      case "codespan":
        return <code key={key}>{(token as Tokens.Codespan).text}</code>;
      case "code":
        return (
          <CodeBlock key={key} label={`Code block ${key + 1}`}>
            {(token as Tokens.Code).text}
          </CodeBlock>
        );
      case "link": {
        const value = token as Tokens.Link;
        return (
          <span key={key}>
            {renderTokens(value.tokens)}
            <small> ({value.href})</small>
          </span>
        );
      }
      case "image": {
        const value = token as Tokens.Image;
        return (
          <span key={key}>
            [Image: {value.text}] ({value.href})
          </span>
        );
      }
      case "list": {
        const value = token as Tokens.List;
        const rows = value.items.map((item, index) => (
          <li key={index}>
            {item.task ? (item.checked ? "[x] " : "[ ] ") : ""}
            {renderTokens(item.tokens)}
          </li>
        ));
        return value.ordered ? (
          <ol
            key={key}
            start={typeof value.start === "number" ? value.start : undefined}
          >
            {rows}
          </ol>
        ) : (
          <ul key={key}>{rows}</ul>
        );
      }
      case "table": {
        const value = token as Tokens.Table;
        return (
          <ScrollRegion key={key} label={`Markdown table ${key + 1}`}>
            <table>
              <thead>
                <tr>
                  {value.header.map((cell, index) => (
                    <th key={index}>{renderTokens(cell.tokens)}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {value.rows.map((row, index) => (
                  <tr key={index}>
                    {row.map((cell, column) => (
                      <td key={column}>{renderTokens(cell.tokens)}</td>
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
      <Metadata>Links and embedded content are displayed as text.</Metadata>
    </Panel>
  );
}

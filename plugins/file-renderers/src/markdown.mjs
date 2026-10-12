import { Lexer } from "marked";

const output = document.getElementById("output");
const title = document.getElementById("title");
const scope = document.getElementById("scope");
const status = document.getElementById("status");
const refresh = document.getElementById("refresh");
let port;
let pending = false;

function node(tag, text) {
  const element = document.createElement(tag);
  if (text !== undefined) element.textContent = text;
  return element;
}

// Marked owns parsing; the closed DOM projection never admits authored HTML,
// URL attributes, scripts or embeds into the isolated document.
function renderTokens(tokens) {
  const fragment = document.createDocumentFragment();
  for (const token of tokens) {
    let element;
    switch (token.type) {
      case "space":
      case "def":
        continue;
      case "hr":
        element = node("hr");
        break;
      case "br":
        element = node("br");
        break;
      case "heading":
        element = node(["h1", "h2", "h3", "h4", "h5", "h6"][token.depth - 1]);
        break;
      case "paragraph":
        element = node("p");
        break;
      case "strong":
        element = node("strong");
        break;
      case "em":
        element = node("em");
        break;
      case "del":
        element = node("del");
        break;
      case "blockquote":
        element = node("blockquote");
        break;
      case "codespan":
        element = node("code", token.text);
        break;
      case "code":
        element = node("pre");
        element.append(node("code", token.text));
        break;
      case "link":
        element = node("span");
        element.append(renderTokens(token.tokens));
        element.append(node("small", ` (${token.href})`));
        break;
      case "image":
        element = node("span", `[Image: ${token.text}] (${token.href})`);
        break;
      case "list":
        element = node(token.ordered ? "ol" : "ul");
        if (token.ordered) element.start = token.start;
        for (const item of token.items) {
          const row = node("li");
          if (item.task)
            row.append(document.createTextNode(item.checked ? "[x] " : "[ ] "));
          row.append(renderTokens(item.tokens));
          element.append(row);
        }
        break;
      case "table": {
        element = node("table");
        const head = node("thead");
        const header = node("tr");
        for (const cell of token.header) {
          const value = node("th");
          value.append(renderTokens(cell.tokens));
          header.append(value);
        }
        head.append(header);
        element.append(head);
        const body = node("tbody");
        for (const cells of token.rows) {
          const row = node("tr");
          for (const cell of cells) {
            const value = node("td");
            value.append(renderTokens(cell.tokens));
            row.append(value);
          }
          body.append(row);
        }
        element.append(body);
        break;
      }
      case "text":
        if (token.tokens) fragment.append(renderTokens(token.tokens));
        else fragment.append(document.createTextNode(token.text));
        continue;
      case "html":
      case "escape":
        fragment.append(document.createTextNode(token.text));
        continue;
      default:
        fragment.append(document.createTextNode(token.raw ?? token.text ?? ""));
        continue;
    }
    if (token.tokens && !["link", "code", "codespan"].includes(token.type))
      element.append(renderTokens(token.tokens));
    fragment.append(element);
  }
  return fragment;
}

function accept(value) {
  const file = value?.file;
  if (
    value?.format !== "flame.file.v1" ||
    !file ||
    typeof file.content !== "string" ||
    typeof file.path !== "string" ||
    file.startLine !== 1 ||
    file.truncated === true ||
    !Number.isSafeInteger(file.totalLines) ||
    file.totalLines < 0
  ) {
    port.postMessage({
      type: "failure",
      reason: "Unsupported captured file format.",
    });
    return;
  }
  let content;
  try {
    content = renderTokens(Lexer.lex(file.content, { gfm: true }));
  } catch {
    port.postMessage({
      type: "failure",
      reason: "The captured Markdown file could not be displayed.",
    });
    return;
  }
  title.textContent = file.path;
  scope.textContent = `${file.cwd ?? "Runtime default workspace"} · ${file.totalLines} recorded lines`;
  output.replaceChildren(content);
  status.setAttribute("role", "status");
  status.textContent =
    "Captured read-only file. Links and embedded content are displayed as text.";
  pending = false;
  refresh.disabled = false;
}

addEventListener("message", (event) => {
  if (
    event.source !== parent ||
    port ||
    event.data?.type !== "flame.view.connect.v3" ||
    !["light", "dark"].includes(event.data.theme?.scheme) ||
    event.ports.length !== 1
  )
    return;
  port = event.ports[0];
  port.onmessage = (message) => {
    if (message.data.type === "data" && pending) accept(message.data.data);
    if (message.data.type === "error" && pending) {
      pending = false;
      refresh.disabled = false;
      status.setAttribute("role", "alert");
      status.textContent = message.data.reason;
    }
  };
  port.start();
  accept(event.data.initial);
});
parent.postMessage("flame.view.ready.v3", "*");
refresh.addEventListener("click", () => {
  if (!port || pending) return;
  pending = true;
  refresh.disabled = true;
  port.postMessage({ type: "read" });
});

const THEME = new Map([
  ["--pele-bg", "--color-surface-2"],
  ["--pele-fg", "--color-text"],
  ["--pele-line", "--color-text-faint"],
  ["--pele-accent", "--color-accent"],
  ["--pele-muted", "--color-text-muted"],
  ["--pele-surface", "--color-surface-2"],
  ["--pele-surface-alt", "--color-surface-3"],
  ["--pele-border", "--color-border"],
]);

function svgRoot(svg: string): Element {
  const root = new DOMParser().parseFromString(svg, "image/svg+xml").documentElement;
  if (root.namespaceURI !== "http://www.w3.org/2000/svg" || root.localName !== "svg") {
    throw new Error("Mermaid renderer returned invalid SVG");
  }
  return root;
}

export async function renderMermaid(code: string, idPrefix: string): Promise<string> {
  const { renderAsync } = await import("pele/lazy");
  const { svg } = await renderAsync(code, {
    idPrefix,
    fontFamily: "system-ui",
    autoDirection: false,
    links: false,
    images: false,
  });
  const root = svgRoot(svg);
  const theme = Array.from(THEME, ([name, value]) => `${name}:var(${value})`).join(";");
  root.setAttribute("style", `${root.getAttribute("style") ?? ""};${theme};background:transparent`);
  return new XMLSerializer().serializeToString(root);
}

// The lightbox and transcript coexist, so each copy needs its own SVG fragment IDs.
export function copyMermaidSvg(svg: string, prefix: string): string {
  const root = svgRoot(svg);
  const ids = new Map<string, string>();
  for (const node of [root, ...root.querySelectorAll("[id]")]) {
    if (!node.hasAttribute("id")) continue;
    const id = node.id;
    ids.set(id, `${prefix}-${id}`);
    node.id = ids.get(id)!;
  }
  for (const node of [root, ...root.querySelectorAll("*")]) {
    for (const attribute of Array.from(node.attributes)) {
      const value = attribute.value.replace(/url\(#([^)]+)\)/g, (original, id: string) =>
        ids.has(id) ? `url(#${ids.get(id)})` : original,
      );
      if (attribute.localName === "href" && value.startsWith("#") && ids.has(value.slice(1))) {
        attribute.value = `#${ids.get(value.slice(1))}`;
      } else if (["aria-labelledby", "aria-describedby"].includes(attribute.name)) {
        attribute.value = value
          .split(/\s+/)
          .map((id) => ids.get(id) ?? id)
          .join(" ");
      } else {
        attribute.value = value;
      }
    }
  }
  return new XMLSerializer().serializeToString(root);
}

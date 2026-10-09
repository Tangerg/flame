// Source SVGs deliberately support paths only; unsupported drawing features must fail the build.
export function readSvg(text, grid, file) {
  const fail = (reason) => {
    throw new Error(`${file}: ${reason}`);
  };
  function attributes(text, allowed) {
    const attributes = {};
    const residue = text.replace(/\s+([\w-]+)="([^"]*)"/g, (_, key, value) => {
      if (!allowed.includes(key) || key in attributes)
        fail(`unsupported or repeated attribute ${key}`);
      attributes[key] = value;
      return "";
    });
    if (residue.trim()) fail("invalid attributes");
    return attributes;
  }
  const root = /^\s*<svg([^>]*)>([\s\S]*?)<\/svg>\s*$/.exec(text);
  if (!root) fail("expected one svg element");
  const attrs = attributes(root[1], [
    "xmlns",
    "viewBox",
    "fill",
    "stroke",
    "stroke-width",
    "stroke-linecap",
    "stroke-linejoin",
  ]);
  const expected = {
    xmlns: "http://www.w3.org/2000/svg",
    viewBox: `0 0 ${grid} ${grid}`,
    fill: "none",
    stroke: "currentColor",
    "stroke-linecap": "round",
    "stroke-linejoin": "round",
  };
  for (const [key, value] of Object.entries(expected)) {
    if (attrs[key] !== value) fail(`expected ${key}="${value}"`);
  }
  const strokeWidth = Number(attrs["stroke-width"]);
  if (!Number.isFinite(strokeWidth) || strokeWidth <= 0)
    fail("invalid stroke width");
  const paths = [];
  const residue = root[2].replace(/<path([^>]*)\/>/g, (_, text) => {
    const attrs = attributes(text, ["d", "stroke-width"]);
    if (!attrs.d || !/^[MmLlHhVvCcSsQqTtAaZz0-9.,+\s-]+$/.test(attrs.d))
      fail("invalid path data");
    const path = { d: attrs.d };
    if (attrs["stroke-width"] !== undefined) {
      path.strokeWidth = Number(attrs["stroke-width"]);
      if (!Number.isFinite(path.strokeWidth) || path.strokeWidth <= 0)
        fail("invalid path stroke width");
    }
    paths.push(path);
    return "";
  });
  if (residue.trim() || !paths.length)
    fail("expected nonempty path-only drawing");
  return { grid, strokeWidth, paths };
}

export function writeSvg(master) {
  const { grid, strokeWidth, paths } = master;
  const body = paths
    .map(
      ({ d, strokeWidth }) =>
        `  <path d="${d}"${strokeWidth === undefined ? "" : ` stroke-width="${strokeWidth}"`}/>`,
    )
    .join("\n");
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${grid} ${grid}" width="${grid}" height="${grid}" fill="none" stroke="currentColor" stroke-width="${strokeWidth}" stroke-linecap="round" stroke-linejoin="round">\n${body}\n</svg>\n`;
}

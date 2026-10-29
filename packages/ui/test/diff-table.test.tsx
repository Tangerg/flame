import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { readFileSync } from "node:fs";
import { DiffTable, type ComparisonRow } from "../dist/index.js";

const rows: ComparisonRow[] = [
  { type: "hunk", text: "@@ -1,2 +1,2 @@" },
  { type: "context", code: "package review", leftLine: 1, rightLine: 1 },
  { type: "deleted", code: "\told := true", leftLine: 2 },
  { type: "added", code: "\tnext := true", rightLine: 2 },
];

const labels = {
  region: "Difference reader",
  beforeLine: "Original line number",
  beforeCode: "Original source",
  afterLine: "Modified line number",
  afterCode: "Modified source",
  changedCode: "Changed source",
};

function renderDiff(layout: "unified" | "split", source = rows) {
  render(
    <>
      <p>Surrounding conversation text</p>
      <DiffTable
        rows={source}
        layout={layout}
        label="Changes to review.go"
        labels={labels}
      />
      <p>Following workspace text</p>
    </>,
  );
  const region = screen.getByRole("region", { name: labels.region });
  const table = screen.getByRole("table", { name: "Changes to review.go" });
  const [, body] = screen.getAllByRole("rowgroup");
  return { region, table, body };
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  document.getSelection()?.removeAllRanges();
});

it.each(["unified", "split"] as const)(
  "preserves source and missing-newline markers in %s layout",
  (layout) => {
    const { table } = renderDiff(layout, [
      { type: "deleted", leftLine: 7, code: "old", noNewline: true },
      { type: "added", rightLine: 9, code: "new", noNewline: true },
    ]);
    expect(
      Array.from(table.querySelectorAll("code"), (code) => code.textContent),
    ).toEqual([
      "old\n\\ No newline at end of file",
      "new\n\\ No newline at end of file",
    ]);
  },
);

describe("DiffTable Select All", () => {
  it.each([
    { layout: "unified", modifier: { ctrlKey: true } },
    { layout: "unified", modifier: { metaKey: true } },
    { layout: "split", modifier: { ctrlKey: true } },
    { layout: "split", modifier: { metaKey: true } },
  ] as const)(
    "selects only the $layout difference rows with $modifier",
    ({ layout, modifier }) => {
      const { region, table, body } = renderDiff(layout);
      const selection = document.getSelection()!;
      region.focus();

      expect(fireEvent.keyDown(region, { key: "a", ...modifier })).toBe(false);
      const range = selection.getRangeAt(0);
      expect(range.startContainer).toBe(body);
      expect(range.endContainer).toBe(body);
      const selected = selection.toString();
      for (const source of [
        "@@ -1,2 +1,2 @@",
        "package review",
        "\told := true",
        "\tnext := true",
      ])
        expect(selected).toContain(source);
      for (const framing of [
        "Surrounding conversation text",
        "Following workspace text",
        "Changes to review.go",
        ...Object.values(labels),
      ])
        expect(selected).not.toContain(framing);

      expect(document.activeElement).toBe(region);
      expect(table.dataset.layout).toBe(layout);
      expect(screen.getAllByRole("columnheader")).toHaveLength(
        layout === "split" ? 4 : 3,
      );
    },
  );

  it.each([{ isComposing: true }, { keyCode: 229 }])(
    "leaves composition %o to the input method",
    (composition) => {
      const { region } = renderDiff("unified");
      region.focus();
      expect(
        fireEvent.keyDown(region, { key: "a", metaKey: true, ...composition }),
      ).toBe(true);
      expect(document.getSelection()?.rangeCount).toBe(0);
    },
  );

  it("does not replace a Select All an earlier handler already owns", () => {
    render(
      <div onKeyDownCapture={(event) => event.preventDefault()}>
        <DiffTable
          rows={rows}
          layout="split"
          label="Changes to review.go"
          labels={labels}
        />
      </div>,
    );
    const region = screen.getByRole("region", { name: labels.region });
    region.focus();
    fireEvent.keyDown(region, { key: "a", ctrlKey: true });
    expect(document.getSelection()?.rangeCount).toBe(0);
  });

  it("keeps a partial selection and its copy native", () => {
    const { region } = renderDiff("split");
    const selection = document.getSelection()!;
    const deleted = screen.getByText(
      (_, element) =>
        element?.tagName === "CODE" && element.textContent === "\told := true",
    );
    region.focus();
    selection.selectAllChildren(deleted);

    expect(fireEvent.keyDown(region, { key: "c", metaKey: true })).toBe(true);
    const setData = vi.fn();
    expect(fireEvent.copy(deleted, { clipboardData: { setData } })).toBe(true);
    expect(setData).not.toHaveBeenCalled();
    expect(selection.toString()).toBe("\told := true");
  });

  it("leaves copying the selected difference rows to the platform", () => {
    const { region } = renderDiff("unified");
    region.focus();
    fireEvent.keyDown(region, { key: "a", metaKey: true });
    const setData = vi.fn();
    expect(fireEvent.copy(region, { clipboardData: { setData } })).toBe(true);
    expect(setData).not.toHaveBeenCalled();
  });
});

describe("DiffTable replacement reading", () => {
  const replacements: ComparisonRow[] = [
    { type: "hunk", text: "@@ -1,4 +1,3 @@" },
    { type: "context", code: "function read() {", leftLine: 1, rightLine: 1 },
    { type: "deleted", code: "  const count = 1;", leftLine: 2 },
    { type: "deleted", code: "  return oldValue + beforeCount;", leftLine: 3 },
    { type: "deleted", code: "  removed();", leftLine: 4 },
    { type: "added", code: "  const count = 2;", rightLine: 2 },
    { type: "added", code: "  return newValue + afterCount;", rightLine: 3 },
  ];

  it("pairs adjacent replacement lines and keeps unmatched deletions on their own side", () => {
    const { body } = renderDiff("split", replacements);
    const source = within(body)
      .getAllByRole("row")
      .map((row) =>
        within(row)
          .getAllByRole("cell")
          .map((cell) => cell.textContent),
      );
    expect(source).toEqual([
      ["@@ -1,4 +1,3 @@"],
      ["1", "function read() {", "1", "function read() {"],
      ["2", "−  const count = 1;", "2", "+  const count = 2;"],
      [
        "3",
        "−  return oldValue + beforeCount;",
        "3",
        "+  return newValue + afterCount;",
      ],
      ["4", "−  removed();", "", ""],
    ]);
  });

  it("never pairs through context or a hunk and keeps unmatched additions", () => {
    const source: ComparisonRow[] = [
      { type: "deleted", code: "previous", leftLine: 0 },
      { type: "hunk", text: "@@ -8,1 +8,1 @@" },
      { type: "added", code: "next", rightLine: 8 },
      { type: "context", code: "stable", leftLine: 9, rightLine: 9 },
      { type: "deleted", code: "one", leftLine: 10 },
      { type: "added", code: "two", rightLine: 10 },
      { type: "added", code: "extra", rightLine: 11 },
    ];
    const { body } = renderDiff("split", source);
    const cells = within(body)
      .getAllByRole("row")
      .map((row) =>
        within(row)
          .getAllByRole("cell")
          .map((cell) => cell.textContent),
      );
    expect(cells).toEqual([
      ["0", "−previous", "", ""],
      ["@@ -8,1 +8,1 @@"],
      ["", "", "8", "+next"],
      ["9", "stable", "9", "stable"],
      ["10", "−one", "10", "+two"],
      ["", "", "11", "+extra"],
    ]);
    expect(body.querySelectorAll("mark")).toHaveLength(2);
  });

  it.each(["unified", "split"] as const)(
    "emphasizes separate changed words in %s without marking shared operators",
    (layout) => {
      const { body } = renderDiff(layout, replacements);
      expect(
        Array.from(body.querySelectorAll("mark"), (mark) => mark.textContent),
      ).toEqual(
        layout === "split"
          ? ["1", "2", "oldValue", "beforeCount", "newValue", "afterCount"]
          : ["1", "oldValue", "beforeCount", "2", "newValue", "afterCount"],
      );
      expect(
        Array.from(body.querySelectorAll("code"), (code) => code.textContent),
      ).toEqual(
        layout === "unified"
          ? replacements
              .filter((row) => row.type !== "hunk")
              .map((row) => row.code)
          : [
              "function read() {",
              "function read() {",
              "  const count = 1;",
              "  const count = 2;",
              "  return oldValue + beforeCount;",
              "  return newValue + afterCount;",
              "  removed();",
              "",
            ],
      );
    },
  );

  it("preserves source order in unified layout when one change block follows another", () => {
    const source: ComparisonRow[] = [
      { type: "added", code: "leading addition", rightLine: 1 },
      ...replacements.slice(2),
      { type: "deleted", code: "following deletion", leftLine: 5 },
    ];
    const { body } = renderDiff("unified", source);
    expect(
      Array.from(body.querySelectorAll("code"), (code) => code.textContent),
    ).toEqual(source.map((row) => (row.type === "hunk" ? row.text : row.code)));
  });

  it("keeps whitespace, punctuation and complete Unicode graphemes selectable", () => {
    const before = '\tconst 文案 = "你好 👩🏽‍💻 café";';
    const after = '\tconst 文案 =  "您好 👨🏾‍💻 café";';
    const { body } = renderDiff("split", [
      { type: "deleted", code: before, leftLine: 1 },
      { type: "added", code: after, rightLine: 1 },
    ]);
    const codes = Array.from(body.querySelectorAll("code"));
    expect(codes.map((code) => code.textContent)).toEqual([before, after]);
    expect(body.querySelectorAll("mark").length).toBeGreaterThan(1);
    const segmenter = new Intl.Segmenter("und", { granularity: "grapheme" });
    for (const code of codes) {
      const text = code.textContent!;
      const boundaries = new Set(
        [...segmenter.segment(text)].map(({ index }) => index),
      );
      boundaries.add(text.length);
      let offset = 0;
      for (const child of code.childNodes) {
        expect(boundaries.has(offset)).toBe(true);
        offset += child.textContent!.length;
        expect(boundaries.has(offset)).toBe(true);
      }
    }
  });

  it("retains complete complex source lines when fine-grained emphasis reaches its bound", () => {
    const before = "old ".repeat(1000);
    const after = "new ".repeat(1000);
    const { body } = renderDiff("split", [
      { type: "deleted", code: before, leftLine: 1 },
      { type: "added", code: after, rightLine: 1 },
    ]);
    expect(
      Array.from(body.querySelectorAll("code"), (code) => code.textContent),
    ).toEqual([before, after]);
    expect(body.querySelectorAll("mark")).toHaveLength(2);
  });

  it("keeps empty and identical replacement lines without invented word changes", () => {
    const { body } = renderDiff("split", [
      { type: "deleted", code: "", leftLine: 1 },
      { type: "deleted", code: "unchanged text", leftLine: 2 },
      { type: "added", code: "", rightLine: 1 },
      { type: "added", code: "unchanged text", rightLine: 2 },
    ]);
    expect(within(body).getAllByRole("row")).toHaveLength(2);
    expect(
      Array.from(body.querySelectorAll("code"), (code) => code.textContent),
    ).toEqual(["", "", "unchanged text", "unchanged text"]);
    expect(body.querySelectorAll("mark")).toHaveLength(0);
  });

  it("preserves the named reader and its focus and scroll when layout changes", () => {
    const view = render(
      <DiffTable
        rows={replacements}
        layout="unified"
        label="Review"
        labels={labels}
      />,
    );
    const region = screen.getByRole("region", { name: labels.region });
    region.focus();
    region.scrollLeft = 64;
    region.scrollTop = 40;
    view.rerender(
      <DiffTable
        rows={replacements}
        layout="split"
        label="Review"
        labels={labels}
      />,
    );
    expect(screen.getByRole("region", { name: labels.region })).toBe(region);
    expect(document.activeElement).toBe(region);
    expect(region.scrollLeft).toBe(64);
    expect(region.scrollTop).toBe(40);
    expect(screen.getAllByRole("columnheader")).toHaveLength(4);
  });

  it("distributes a visible changed-word cue that does not rely on color", () => {
    const stylesheet = document.createElement("style");
    stylesheet.textContent = readFileSync("dist/styles.css", "utf8");
    document.head.append(stylesheet);
    try {
      const { body } = renderDiff("split", replacements);
      const changed = body.querySelector("mark")!;
      expect(
        getComputedStyle(changed).getPropertyValue("border-bottom-width"),
      ).toBe("1px");
      expect(
        getComputedStyle(changed).getPropertyValue("border-bottom-style"),
      ).toBe("solid");
    } finally {
      stylesheet.remove();
    }
  });

  it("keeps the first visible source line anchored across both layout changes", () => {
    const source: ComparisonRow[] = Array.from({ length: 30 }, (_, index) => {
      const line = index + 1;
      return line % 2 === 0
        ? [
            { type: "deleted" as const, code: `old${line}`, leftLine: line },
            { type: "added" as const, code: `new${line}`, rightLine: line },
          ]
        : [
            {
              type: "context" as const,
              code: `stable${line}`,
              leftLine: line,
              rightLine: line,
            },
          ];
    }).flat();
    const content = (layout: "unified" | "split") => (
      <div style={{ overflowY: "auto" }} data-testid="viewport">
        <DiffTable
          rows={source}
          layout={layout}
          label="Review"
          labels={labels}
        />
      </div>
    );
    const view = render(content("unified"));
    const viewport = screen.getByTestId("viewport");
    Object.defineProperties(viewport, {
      clientHeight: { value: 200 },
      scrollHeight: {
        get: () => viewport.querySelector("tbody")!.children.length * 20,
      },
    });
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
      function (this: HTMLElement) {
        if (this === viewport) return new DOMRect(0, 100, 600, 200);
        if (this.tagName === "TD") {
          const row = this.parentElement as HTMLTableRowElement;
          const position = Array.from(row.parentElement!.children).indexOf(row);
          return new DOMRect(
            0,
            100 + position * 20 - viewport.scrollTop,
            600,
            20,
          );
        }
        return new DOMRect(0, 0, 600, 768);
      },
    );
    viewport.scrollTop = 140;
    const cell = () =>
      Array.from(viewport.querySelectorAll("td")).find(
        (cell) => cell.querySelector("code")?.textContent === "old6",
      )!;
    const before = cell().getBoundingClientRect().top;
    expect(before).toBe(100);
    view.rerender(content("split"));
    expect(cell().getBoundingClientRect().top).toBe(before);
    view.rerender(content("unified"));
    expect(cell().getBoundingClientRect().top).toBe(before);
  });
});

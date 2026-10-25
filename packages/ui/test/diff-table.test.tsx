import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
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

function renderDiff(layout: "unified" | "split") {
  render(
    <>
      <p>Surrounding conversation text</p>
      <DiffTable
        rows={rows}
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
  document.getSelection()?.removeAllRanges();
});

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
    const deleted = screen.getByText("old := true");
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

import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { ChartView } from "../../../plugins/genui/component/chart";

afterEach(cleanup);

const comparisonSeries = Array.from({ length: 12 }, (_, index) => ({
  label: `Evidence ${index + 1}`,
  values: [index + 1, index + 2],
}));

function markerSignature(element: Element | null) {
  const selector = "circle,rect,path,polygon";
  const shape = element?.matches(selector) ? element : element?.querySelector(selector);
  if (!shape) return undefined;
  const paint = (name: string) => {
    for (let node: Element | null = shape; node; node = node.parentElement) {
      if (node.hasAttribute(name)) return node.getAttribute(name);
    }
    return null;
  };
  return JSON.stringify({
    shape: shape.localName,
    geometry: ["d", "points", "r", "width", "height"].map((name) => shape.getAttribute(name)),
    fill: paint("fill"),
    stroke: paint("stroke"),
    strokeWidth: paint("stroke-width"),
  });
}

it.each(["scatter", "line", "area"] as const)(
  "keeps all twelve %s series distinct and matches their markers to the data legend",
  (kind) => {
    render(
      <ChartView
        kind={kind}
        caption="Evidence comparison"
        labels={["A", "B"]}
        xValues={[10, 110]}
        series={comparisonSeries}
        dataLabel="Comparison data"
        emptyLabel="No data"
      />,
    );
    const chart = screen.getByRole("img", { name: "Evidence comparison" });
    const table = screen.getByRole("table", { name: "Comparison data" });
    const signatures = comparisonSeries.map((series) => {
      const point = within(chart).getByText(`${series.label} · A: ${series.values[0]}`, {
        selector: "title",
      }).parentElement;
      const marker = markerSignature(point);
      const header = within(table).getByRole("columnheader", { name: series.label });
      expect(marker).toBeDefined();
      expect(markerSignature(header.querySelector("svg"))).toBe(marker);
      return marker;
    });
    expect(new Set(signatures).size).toBe(12);
    expect(within(table).getByRole("cell", { name: "13" })).toBeTruthy();
  },
);

it("matches each grouped-bar fill to its own data legend and keeps all twelve series distinct", () => {
  const { container } = render(
    <ChartView
      kind="bar"
      caption="Evidence comparison"
      labels={["A", "B"]}
      series={comparisonSeries}
      dataLabel="Comparison data"
      emptyLabel="No data"
    />,
  );
  const chart = screen.getByRole("img", { name: "Evidence comparison" });
  const table = screen.getByRole("table", { name: "Comparison data" });
  const patterns = comparisonSeries.map((series) => {
    const bar = within(chart).getByText(`${series.label} · A: ${series.values[0]}`, {
      selector: "title",
    }).parentElement;
    expect(bar?.localName).toBe("rect");
    const fill = bar?.getAttribute("fill");
    expect(fill).toMatch(/^url\(#.+\)$/);
    const header = within(table).getByRole("columnheader", { name: series.label });
    expect(header.querySelector("svg rect")?.getAttribute("fill")).toBe(fill);
    const id = fill!.slice(5, -1);
    const pattern = container.querySelector(`[id="${id}"]`);
    expect(pattern?.localName).toBe("pattern");
    return ["width", "height", "patternTransform"]
      .map((name) => pattern!.getAttribute(name))
      .join(";");
  });
  expect(new Set(patterns).size).toBe(12);
  expect(within(table).getByRole("cell", { name: "13" })).toBeTruthy();
});

it("provides separate named keyboard reading regions for the plot and its exact data", () => {
  render(
    <ChartView
      kind="line"
      caption="Evidence comparison"
      labels={["A", "B"]}
      series={comparisonSeries}
      dataLabel="Comparison data"
      emptyLabel="No data"
    />,
  );
  const plot = screen.getByRole("region", { name: "Evidence comparison" });
  const data = screen.getByRole("region", { name: "Comparison data" });
  expect(plot.tabIndex).toBe(0);
  expect(within(plot).getByRole("img", { name: "Evidence comparison" })).toBeTruthy();
  expect(data.tabIndex).toBe(0);
  expect(within(data).getByRole("table", { name: "Comparison data" })).toBeTruthy();
  expect(data).not.toBe(plot);
});

it.each(["line", "bar", "area", "pie", "donut"] as const)(
  "renders %s geometry with complete readable data and literal labels",
  (kind) => {
    const { container } = render(
      <ChartView
        kind={kind}
        caption="Quarterly volume"
        labels={["Q1 <script>", "Q2", "Q3"]}
        series={[{ label: "Orders", values: [12, 24, 18] }]}
        dataLabel="Quarterly volume data"
        emptyLabel="No data"
      />,
    );
    expect(screen.getByRole("img", { name: "Quarterly volume" })).toBeTruthy();
    const table = screen.getByRole("table", { name: "Quarterly volume data" });
    expect(within(table).getByRole("columnheader", { name: "Orders" })).toBeTruthy();
    expect(within(table).getByRole("rowheader", { name: "Q1 <script>" })).toBeTruthy();
    expect(
      within(table)
        .getAllByRole("cell")
        .map((cell) => cell.textContent),
    ).toEqual(
      kind === "pie" || kind === "donut"
        ? ["12", "22.22%", "24", "44.44%", "18", "33.33%"]
        : ["12", "24", "18"],
    );
    expect(container.querySelector("script")).toBeNull();
    expect(container.querySelector("svg")?.outerHTML).not.toMatch(/NaN|Infinity/);
    expect(screen.getByRole("region", { name: "Quarterly volume data" }).tabIndex).toBe(0);
  },
);

it("plots scatter points on their numeric x coordinates and exposes both axes as exact data", () => {
  const { container } = render(
    <ChartView
      kind="scatter"
      caption="Latency observations"
      labels={["A", "B", "C"]}
      xValues={[10, 20, 110]}
      series={[{ label: "Latency", values: [3, -2, 5] }]}
      dataLabel="Latency data"
      emptyLabel="No data"
    />,
  );
  const points = [...container.querySelectorAll("circle")].map((point) =>
    Number(point.getAttribute("cx")),
  );
  expect((points[1]! - points[0]!) / (points[2]! - points[0]!)).toBeCloseTo(0.1);
  expect(
    within(screen.getByRole("table", { name: "Latency data" }))
      .getAllByRole("cell")
      .map((cell) => cell.textContent),
  ).toEqual(["10", "3", "20", "-2", "110", "5"]);
  expect(container.querySelector("svg")?.outerHTML).not.toMatch(/NaN|Infinity/);
});

it("plots signed and all-zero Cartesian values without losing their exact data-table values", () => {
  const { container, rerender } = render(
    <ChartView
      kind="area"
      caption="Net changes"
      labels={["First", "Second"]}
      series={[{ label: "Change", values: [-4.25, 3.5] }]}
      dataLabel="Net changes data"
      emptyLabel="No data"
    />,
  );
  expect(screen.getByRole("cell", { name: "-4.25" })).toBeTruthy();
  expect(screen.getByRole("cell", { name: "3.5" })).toBeTruthy();
  expect(container.querySelector("polygon")?.getAttribute("points")).not.toMatch(/NaN|Infinity/);
  rerender(
    <ChartView
      kind="line"
      caption="Net changes"
      labels={["First", "Second"]}
      series={[{ label: "Change", values: [0, 0] }]}
      dataLabel="Net changes data"
      emptyLabel="No data"
    />,
  );
  expect(screen.getAllByRole("cell", { name: "0" })).toHaveLength(2);
  expect(container.querySelector("svg")?.outerHTML).not.toMatch(/NaN|Infinity/);
});

it("handles a whole-circle pie and a zero-total pie without inventing a value", () => {
  const props = {
    kind: "pie" as const,
    caption: "Coverage",
    labels: ["Covered"],
    dataLabel: "Coverage data",
    emptyLabel: "No data",
  };
  const { container, rerender } = render(
    <ChartView {...props} series={[{ label: "Count", values: [5] }]} />,
  );
  expect(screen.getByText("Covered: 5 (100%)")).toBeTruthy();
  expect(container.querySelector("circle")).toBeTruthy();
  rerender(<ChartView {...props} series={[{ label: "Count", values: [0] }]} />);
  expect(screen.getByRole("cell", { name: "0" })).toBeTruthy();
  expect(container.querySelector("circle")).toBeNull();
  expect(container.querySelector("svg")?.outerHTML).not.toMatch(/NaN|Infinity/);
});

it.each(["pie", "donut"] as const)(
  "keeps all 256 %s categories in a complete legend with matching slice patterns and exact values",
  (kind) => {
    const labels = Array.from({ length: 256 }, (_, index) =>
      index === 255
        ? "Final category — complete captured name <literal>"
        : `Captured category ${index + 1}`,
    );
    render(
      <ChartView
        kind={kind}
        caption="Captured composition"
        labels={labels}
        series={[{ label: "Captured values", values: labels.map((_, index) => index + 1) }]}
        dataLabel="Composition data"
        emptyLabel="No data"
      />,
    );
    const chart = screen.getByRole("img", { name: "Captured composition" });
    const table = screen.getByRole("table", { name: "Composition data" });
    const rows = within(table).getAllByRole("row").slice(1);
    const slices = [...chart.querySelectorAll("path,circle")].filter((shape) =>
      shape.querySelector("title"),
    );
    expect(rows).toHaveLength(256);
    expect(slices).toHaveLength(256);
    expect(within(table).getByRole("columnheader", { name: "%" })).toBeTruthy();
    rows.forEach((row, index) => {
      const header = within(row).getByRole("rowheader", { name: labels[index]! });
      const swatch = header.querySelector("svg rect");
      expect(swatch?.getAttribute("fill")).toBe(slices[index]!.getAttribute("fill"));
      expect(swatch?.closest("svg")?.getAttribute("aria-hidden")).toBe("true");
      expect(within(row).getByRole("cell", { name: String(index + 1) })).toBeTruthy();
    });
    expect(within(rows[255]!).getByRole("cell", { name: "0.7782%" })).toBeTruthy();
  },
);

it.each(["pie", "donut"] as const)(
  "keeps %s shares finite and consistent with its geometry through a zero-total update",
  (kind) => {
    const props = {
      kind,
      caption: "Captured composition",
      labels: ["First", "Second", "Zero"],
      dataLabel: "Composition data",
      emptyLabel: "No data",
    };
    const { rerender } = render(
      <ChartView {...props} series={[{ label: "Values", values: [1e308, 1e308, 0] }]} />,
    );
    const values = () =>
      within(screen.getByRole("table", { name: "Composition data" }))
        .getAllByRole("cell")
        .map((cell) => cell.textContent);
    expect(values()).toEqual(["1e+308", "50%", "1e+308", "50%", "0", "0%"]);
    expect(screen.getByText("First: 1e+308 (50%)", { selector: "title" })).toBeTruthy();
    expect(screen.getByRole("img").outerHTML).not.toMatch(/NaN|Infinity/);

    rerender(<ChartView {...props} series={[{ label: "Values", values: [0, 0, 0] }]} />);
    expect(values()).toEqual(["0", "0%", "0", "0%", "0", "0%"]);
    expect(screen.getByRole("img").querySelectorAll("title")).toHaveLength(0);
    expect(screen.getByRole("img").outerHTML).not.toMatch(/NaN|Infinity/);
  },
);

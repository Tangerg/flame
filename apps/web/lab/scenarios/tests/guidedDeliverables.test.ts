import { expect, it } from "vitest";
import { guidedContent } from "../../../../../plugins/deliverables/component/authoring";

it("refuses lossy guided workbook numbers before they become native cells", () => {
  expect(guidedContent("xlsx", "Numbers", "12.5\t00123\t=SUM(A1:A2)", "Data")).toMatchObject({
    sheets: [
      {
        cells: [
          { value: { type: "number", value: 12.5 } },
          { value: { type: "string", value: "00123" } },
          { value: { type: "formula", value: "=SUM(A1:A2)" } },
        ],
      },
    ],
  });
  for (const value of ["9007199254740993", "1.0000000000000001", "1e999"])
    expect(() => guidedContent("xlsx", "Numbers", value, "Data")).toThrow("exactly representable");
});

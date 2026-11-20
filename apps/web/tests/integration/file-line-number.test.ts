import { expect, it } from "vitest";
import { parseFileLineNumber } from "@/features/workspace/ui/files/fileLineNumber";

it.each<[string, number]>([
  ["64", 64],
  ["６４", 64],
  [" ６4 ", 64],
  ["9007199254740991", Number.MAX_SAFE_INTEGER],
])("accepts %j as line %d", (input, line) => {
  expect(parseFileLineNumber(input)).toBe(line);
});

it.each([
  "",
  "０",
  "+６４",
  "-64",
  "6.4",
  "6e4",
  "６ ４",
  "⑥④",
  "٦٤",
  "９００７１９９２５４７４０９９２",
])("rejects %j as a line number", (input) => {
  expect(parseFileLineNumber(input)).toBeNull();
});

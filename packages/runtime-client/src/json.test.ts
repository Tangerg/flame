import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { ExactNumber, parseJSON, stringifyJSON } from "./json";

describe("feature JSON values", () => {
  it("round-trips the same feature numeric corpus as the Go transport", () => {
    const source = readFileSync(
      new URL("../testing/generated/plugin-values.json", import.meta.url),
      "utf8",
    ).trim();
    expect(stringifyJSON(parseJSON(source))).toBe(source);
  });
  it.each([
    "18446744073709551615",
    "-9223372036854775808",
    "9007199254740993",
    "1e20",
    "1e999999999",
    "0.1000000000000000000001",
    "9007199254740993.1",
    "1.0",
    "-0",
  ])("preserves the original numeric literal %s", (number) => {
    const text = `{"number":${number}}`;
    expect(stringifyJSON(parseJSON(text))).toBe(text);
  });
  it("uses bigint for wide integer values and retains ordinary primitives", () => {
    expect(parseJSON("18446744073709551615")).toBe(18446744073709551615n);
    expect(parseJSON("-9223372036854775808")).toBe(-9223372036854775808n);
    expect(parseJSON('{"id":"9007199254740993","value":0.25,"n":1,"unset":null}')).toEqual({
      id: "9007199254740993",
      value: 0.25,
      n: 1,
      unset: null,
    });
    expect(
      stringifyJSON({ revision: 18446744073709551615n, fraction: new ExactNumber("1.0") }),
    ).toBe('{"revision":18446744073709551615,"fraction":1.0}');
  });
  it.each(['{"n":1,"n":2}', '{"value":"\\ud800"}', "undefined", "[1,]", "/* no */ 1"])(
    "refuses ambiguous or malformed JSON %s",
    (text) => {
      expect(() => parseJSON(text)).toThrow(SyntaxError);
    },
  );
  it("preserves own __proto__ members without changing object prototypes", () => {
    const value = parseJSON('{"__proto__":{"n":18446744073709551615}}');
    expect(Object.getPrototypeOf(value)).toBe(Object.prototype);
    expect(stringifyJSON(value)).toBe('{"__proto__":{"n":18446744073709551615}}');
  });
  it("refuses already rounded numbers and unsupported values before dispatch", () => {
    for (const value of [9007199254740992, Infinity, NaN, { fn: () => undefined }, new Map()])
      expect(() => stringifyJSON(value)).toThrow(TypeError);
    expect(() => new ExactNumber("1 2")).toThrow(/EndOfFileExpected/);
    expect(() => new ExactNumber('"1"')).toThrow(TypeError);
    expect(() => new ExactNumber(" 1 ")).toThrow(TypeError);
    expect(() => JSON.stringify(new ExactNumber("1.0"))).toThrow(/stringifyJSON/);
    const cyclic: { self?: unknown } = {};
    cyclic.self = cyclic;
    expect(() => stringifyJSON(cyclic)).toThrow(/cyclic/);
    const holes: unknown[] = Array(3);
    holes[2] = null;
    expect(stringifyJSON({ optional: undefined, values: holes })).toBe(
      '{"values":[null,null,null]}',
    );
  });
});

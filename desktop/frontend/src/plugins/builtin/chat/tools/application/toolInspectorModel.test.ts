import { describe, expect, it } from "vitest";
import { formatToolInspectorBody, toolInspectorModel } from "./toolInspectorModel";

function json(text: string) {
  return { text, isJson: true };
}

function literal(text: string) {
  return { text, isJson: false };
}

describe("formatToolInspectorBody", () => {
  it("pretty prints structured JSON bodies", () => {
    expect(formatToolInspectorBody('{"path":"src/app.ts","lines":[1,2]}')).toEqual(
      json('{\n  "path": "src/app.ts",\n  "lines": [\n    1,\n    2\n  ]\n}'),
    );
  });

  it("keeps every numeric lexeme exactly as the tool wrote it", () => {
    expect(
      formatToolInspectorBody(
        '{"id":9007199254740993,"uint64":18446744073709551615,"price":0.10000000000000000555,' +
          '"amount":1e400,"tiny":-1E-400,"offset":-0,"ratio":1.0,"signed":[-2.50e+10,0]}',
      ),
    ).toEqual(
      json(
        [
          "{",
          '  "id": 9007199254740993,',
          '  "uint64": 18446744073709551615,',
          '  "price": 0.10000000000000000555,',
          '  "amount": 1e400,',
          '  "tiny": -1E-400,',
          '  "offset": -0,',
          '  "ratio": 1.0,',
          '  "signed": [',
          "    -2.50e+10,",
          "    0",
          "  ]",
          "}",
        ].join("\n"),
      ),
    );
  });

  it("keeps every duplicate member in its original order", () => {
    expect(
      formatToolInspectorBody('{"mode":"read","path":"a","mode":"write","mode":"read"}'),
    ).toEqual(
      json('{\n  "mode": "read",\n  "path": "a",\n  "mode": "write",\n  "mode": "read"\n}'),
    );
  });

  it("keeps string escapes and structural characters inside strings verbatim", () => {
    expect(
      formatToolInspectorBody(
        String.raw`{"name":"caf\u00e9 \ud83d\ude00","quote":"say \"hi\"","shape":"{[1, 2]}: x,","path":"C:\\tmp\/out"}`,
      ),
    ).toEqual(
      json(
        [
          "{",
          String.raw`  "name": "caf\u00e9 \ud83d\ude00",`,
          String.raw`  "quote": "say \"hi\"",`,
          String.raw`  "shape": "{[1, 2]}: x,",`,
          String.raw`  "path": "C:\\tmp\/out"`,
          "}",
        ].join("\n"),
      ),
    );
  });

  it("reindents nested structures and existing indentation to two spaces", () => {
    const expected = json(
      '{\n  "a": {\n    "b": [\n      {\n        "c": null\n      },\n      []\n    ],\n    "d": {}\n  }\n}',
    );
    expect(formatToolInspectorBody('{"a":{"b":[{"c":null},[]],"d":{}}}')).toEqual(expected);
    expect(
      formatToolInspectorBody(
        '\n{\n\t"a" :{\n    "b":[ {"c" : null},\n [ ] ],\r\n        "d":{ }\n}}\n',
      ),
    ).toEqual(expected);
    expect(formatToolInspectorBody(expected.text)).toEqual(expected);
  });

  it("changes only whitespace between tokens", () => {
    const source = '[{"k":"v","n":-0.0e-0},true,false,null,"\\t",{"k":1,"k":2}]';
    const { text } = formatToolInspectorBody(source);
    expect(text.replace(/\s/g, "")).toBe(source);
  });

  it("preserves raw text and malformed structured text", () => {
    expect(formatToolInspectorBody("plain output")).toEqual(literal("plain output"));
    expect(formatToolInspectorBody("{not json")).toEqual(literal("{not json"));
  });

  it("keeps lenient JSON dialects as exact literal text", () => {
    for (const source of [
      '{"a":1, // note\n"b":2}',
      '{"a":1 /* note */}',
      '{"a":[1,2,],}',
      "[1,2,]",
      "{'a':1}",
      '{"a":01}',
      '{"a":NaN}',
      ' {"a":1}}\n',
    ]) {
      expect(formatToolInspectorBody(source)).toEqual(literal(source));
    }
  });

  it("keeps scalar JSON documents as literal text", () => {
    expect(formatToolInspectorBody("9007199254740993")).toEqual(literal("9007199254740993"));
    expect(formatToolInspectorBody('"text"')).toEqual(literal('"text"'));
  });

  it("treats empty or whitespace-only bodies as absent", () => {
    expect(formatToolInspectorBody(undefined)).toEqual({ text: "", isJson: false });
    expect(formatToolInspectorBody(" \n\t ")).toEqual({ text: "", isJson: false });
  });
});

describe("toolInspectorModel", () => {
  it("shows the no-result row only for successful tools with no result body", () => {
    expect(toolInspectorModel({ args: "", status: "ok" }).showNoResult).toBe(true);
    expect(toolInspectorModel({ args: "", status: "running" }).showNoResult).toBe(false);
    expect(toolInspectorModel({ args: "", result: '{"ok":true}', status: "ok" }).showNoResult).toBe(
      false,
    );
  });

  it("formats both bodies without changing the tool", () => {
    const tool = Object.freeze({
      args: '{"id":9007199254740993}',
      result: '{"offset":-0,"offset":1e400}',
      status: "ok" as const,
    });
    expect(toolInspectorModel(tool)).toEqual({
      args: json('{\n  "id": 9007199254740993\n}'),
      result: json('{\n  "offset": -0,\n  "offset": 1e400\n}'),
      showNoResult: false,
    });
    expect(tool).toEqual({
      args: '{"id":9007199254740993}',
      result: '{"offset":-0,"offset":1e400}',
      status: "ok",
    });
  });
});

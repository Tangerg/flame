import { stringifyJSON } from "@qimu/sdk/json";
import { applyEdits, format } from "jsonc-parser";

export function prettyJSON(value: unknown): string {
  return formatJSONText(stringifyJSON(value));
}

// Change whitespace alone so displayed evidence retains its numeric lexemes.
export function formatJSONText(source: string): string {
  return applyEdits(
    source,
    format(source, undefined, { tabSize: 2, insertSpaces: true, eol: "\n" }),
  );
}

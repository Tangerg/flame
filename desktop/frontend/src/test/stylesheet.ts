import { readFileSync } from "node:fs";
import { join } from "node:path";

const CSS = readFileSync(join(process.cwd(), "src/styles/globals.css"), "utf8");

export function declaredInCss(property: string): string | undefined {
  return new RegExp(`${property}:\\s*([^;]+);`).exec(CSS)?.[1]?.trim();
}

export function declaredInBlock(selector: string, property: string): string | undefined {
  const pattern = new RegExp(`^\\s*${property}:\\s*([^;]+);`, "m");
  let declared: string | undefined;
  for (
    let at = CSS.indexOf(`${selector} {`);
    at !== -1;
    at = CSS.indexOf(`${selector} {`, at + 1)
  ) {
    const found = pattern.exec(CSS.slice(at, CSS.indexOf("\n}", at)))?.[1];
    if (found !== undefined) declared = found.trim();
  }
  return declared;
}

export function collapse(value: string): string {
  return value.replaceAll(/\s+/g, " ").replaceAll("( ", "(").replaceAll(" )", ")").trim();
}

export function driftAgainstBlock(
  selector: string,
  contributed: Readonly<Record<string, string>>,
): { compared: number; disagreed: string[] } {
  const disagreed: string[] = [];
  let compared = 0;
  for (const [name, value] of Object.entries(contributed)) {
    const declared = declaredInBlock(selector, `--${name}`);
    if (declared === undefined) continue;
    compared++;
    if (collapse(declared) !== collapse(value)) {
      disagreed.push(`--${name}: css=${collapse(declared)} spec=${collapse(value)}`);
    }
  }
  return { compared, disagreed };
}

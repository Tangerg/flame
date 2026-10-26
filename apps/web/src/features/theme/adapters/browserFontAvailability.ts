import { type FontAvailability } from "@/features/theme/application/preferences/systemFonts";

export function createBrowserFontAvailability(document: Document): FontAvailability {
  const measured = new Map<string, number>();

  function textWidth(stack: string, text: string): number {
    const key = `${stack}\0${text}`;
    const remembered = measured.get(key);
    if (remembered !== undefined) return remembered;
    const probe = document.createElement("span");
    probe.style.cssText = `position:absolute;visibility:hidden;white-space:nowrap;font-size:48px;font-family:${stack}`;
    probe.textContent = text;
    document.body.append(probe);
    const width = probe.getBoundingClientRect().width;
    probe.remove();
    measured.set(key, width);
    return width;
  }

  const SHAPE = "mmmiiiWWW@#%";

  function isAvailable(family: string): boolean {
    return (
      textWidth(`"${family}", serif`, SHAPE) !== textWidth("serif", SHAPE) ||
      textWidth(`"${family}", monospace`, SHAPE) !== textWidth("monospace", SHAPE)
    );
  }

  function hasTabularFigures(family: string): boolean {
    const stack = `"${family}", monospace`;
    return textWidth(stack, "0000000000") === textWidth(stack, "1111111111");
  }

  return { isAvailable, hasTabularFigures };
}

import { copyText } from "@flame/ui";
export interface RichClipboardText {
  plainText: string;
  htmlText?: string;
}

export function clipboardCodeElement(document: Document, text: string): HTMLPreElement {
  const pre = document.createElement("pre");
  pre.dir = "ltr";
  const code = document.createElement("code");
  code.textContent = text;
  pre.append(code);
  return pre;
}

export async function copyRichText({ plainText, htmlText }: RichClipboardText): Promise<boolean> {
  if (!plainText || typeof navigator === "undefined" || !navigator.clipboard) return false;
  const clipboard = navigator.clipboard;
  try {
    if (
      htmlText &&
      "write" in clipboard &&
      typeof clipboard.write === "function" &&
      typeof ClipboardItem !== "undefined"
    ) {
      await clipboard.write([
        new ClipboardItem({
          "text/plain": new Blob([plainText], { type: "text/plain" }),
          "text/html": new Blob([htmlText], { type: "text/html" }),
        }),
      ]);
    } else {
      await clipboard.writeText(plainText);
    }
    return true;
  } catch {
    return false;
  }
}

export async function writeToClipboard(
  present: (message: { level: "success" | "error"; message: string }) => void,
  text: string,
  options?: { successLabel?: string; failureLabel?: string },
): Promise<boolean> {
  const ok = await copyText(text);
  if (ok && options?.successLabel) present({ level: "success", message: options.successLabel });
  if (!ok && options?.failureLabel) present({ level: "error", message: options.failureLabel });
  return ok;
}

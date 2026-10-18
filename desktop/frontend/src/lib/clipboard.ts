import { copyText } from "@flame/ui";
import { toast } from "sonner";

export interface RichClipboardText {
  plainText: string;
  htmlText?: string;
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
  text: string,
  options?: { successLabel?: string },
): Promise<boolean> {
  const ok = await copyText(text);
  if (ok && options?.successLabel) toast.success(options.successLabel);
  return ok;
}

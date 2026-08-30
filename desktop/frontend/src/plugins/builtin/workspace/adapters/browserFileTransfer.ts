import type { FileTransferPort } from "../application/ports/fileTransfer";

const OBJECT_URL_REVOCATION_DELAY_MS = 1_000;

// Pure browser mechanism — an anchor with a blob URL, a hidden file input — kept out of the
// application layer, which must not reach for `document`.

function downloadFile(filename: string, content: string, mime: string): void {
  const blob = new Blob([content], { type: mime });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  // Revoking immediately races the download in WebKit; a beat later is safe and
  // still bounded.
  setTimeout(() => URL.revokeObjectURL(url), OBJECT_URL_REVOCATION_DELAY_MS);
}

function pickTextFile(accept: string): Promise<string | null> {
  return new Promise((resolve) => {
    const input = document.createElement("input");
    let pending = true;
    const settle = (text: string | null) => {
      if (!pending) return;
      pending = false;
      input.onchange = null;
      input.removeEventListener("cancel", cancel);
      resolve(text);
    };
    const cancel = () => settle(null);
    input.type = "file";
    input.accept = accept;
    input.addEventListener("cancel", cancel, { once: true });
    input.onchange = () => {
      const file = input.files?.[0];
      if (!file) return settle(null);
      void file.text().then(settle, () => settle(null));
    };
    input.click();
  });
}

export function browserFileTransfer(): FileTransferPort {
  return { download: downloadFile, pickText: pickTextFile };
}

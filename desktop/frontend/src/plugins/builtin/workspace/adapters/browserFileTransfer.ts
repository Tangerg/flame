import { downloadBlob } from "@/platform/download";
import type { FileTransferPort } from "../application/ports/fileTransfer";

function downloadFile(filename: string, content: string, mime: string): void {
  downloadBlob(filename, new Blob([content], { type: mime }));
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

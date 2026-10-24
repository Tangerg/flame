import { type ClientHost } from "@/platform/host";
import { type FileTransferPort } from "@/features/workspace/application/ports/fileTransfer";
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

// Saving belongs to the composed host; cancellation is an ordinary outcome, not a failure.
export function createFileTransfer(host: Pick<ClientHost, "saveTextFile">): FileTransferPort {
  return {
    async saveText(filename, content, mime) {
      await host.saveTextFile(filename, content, mime);
    },
    pickText: pickTextFile,
  };
}

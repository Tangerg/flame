// Cancellation and an empty selection resolve null; a failed read of a selected file rejects.
export function pickTextFile(accept: string): Promise<string | null> {
  return new Promise((resolve, reject) => {
    const input = document.createElement("input");
    let pending = true;
    const settle = (publish: () => void) => {
      if (!pending) return;
      pending = false;
      input.onchange = null;
      input.removeEventListener("cancel", cancel);
      publish();
    };
    const cancel = () => settle(() => resolve(null));
    input.type = "file";
    input.accept = accept;
    input.addEventListener("cancel", cancel, { once: true });
    input.onchange = () => {
      const file = input.files?.[0];
      if (!file) return cancel();
      void file.text().then(
        (text) => settle(() => resolve(text)),
        (error: unknown) => settle(() => reject(error)),
      );
    };
    input.click();
  });
}

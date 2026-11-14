import { type AgentInput, type AgentInputPart } from "@/features/agent/public/input";
export interface InputImage {
  mime: string;
  data: string;
}

export function buildInput(text: string, images: readonly InputImage[]): AgentInput {
  const parts: AgentInputPart[] = [];
  if (text) parts.push({ kind: "text", text });
  for (const img of images) parts.push({ kind: "image", mime: img.mime, data: img.data });
  return { parts };
}

export async function fileToInputImage(
  file: File,
  signal?: AbortSignal,
): Promise<InputImage & { name: string }> {
  const dataUrl = await new Promise<string>((resolve, reject) => {
    if (signal?.aborted) {
      reject(signal.reason);
      return;
    }
    const reader = new FileReader();
    const abort = () => reader.abort();
    const cleanup = () => {
      signal?.removeEventListener("abort", abort);
      reader.onload = reader.onerror = reader.onabort = null;
    };
    reader.onload = () => {
      cleanup();
      resolve(reader.result as string);
    };
    reader.onerror = () => {
      const error = reader.error;
      cleanup();
      reject(error ?? new Error("file read failed"));
    };
    reader.onabort = () => {
      cleanup();
      reject(new DOMException("file read aborted", "AbortError"));
    };
    signal?.addEventListener("abort", abort, { once: true });
    try {
      reader.readAsDataURL(file);
    } catch (cause) {
      cleanup();
      reject(cause);
    }
  });
  const comma = dataUrl.indexOf(",");
  const data = comma >= 0 ? dataUrl.slice(comma + 1) : dataUrl;
  return { mime: file.type, data, name: file.name };
}

export function imageFiles(list: FileList | null | undefined): File[] {
  return list ? Array.from(list).filter((f) => f.type.startsWith("image/")) : [];
}

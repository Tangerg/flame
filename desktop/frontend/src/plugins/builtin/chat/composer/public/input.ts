export interface UserTextInput {
  kind: "text";
  text: string;
}

export interface UserImageInput {
  kind: "image";
  mime: string;
  data: string;
}

export type UserInputPart = UserTextInput | UserImageInput;

export interface UserInput {
  parts: UserInputPart[];
}

/** One image attachment ready to inline. `data` is raw base64 (no data: prefix). */
export interface InputImage {
  mime: string;
  data: string;
}

/** Plain-text input — the common programmatic case. Empty text → empty input. */
export function textInput(text: string): UserInput {
  return text ? { parts: [{ kind: "text", text }] } : { parts: [] };
}

export function buildInput(text: string, images: InputImage[]): UserInput {
  const parts: UserInputPart[] = [];
  if (text) parts.push({ kind: "text", text });
  for (const img of images) parts.push({ kind: "image", mime: img.mime, data: img.data });
  return { parts };
}

/** Into the inline wire form: mime plus raw base64, no `data:` prefix. The caller
 *  pre-filters to image/* files. */
export async function fileToInputImage(file: File): Promise<InputImage & { name: string }> {
  const dataUrl = await new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(reader.error ?? new Error("file read failed"));
    reader.readAsDataURL(file);
  });
  const comma = dataUrl.indexOf(",");
  const data = comma >= 0 ? dataUrl.slice(comma + 1) : dataUrl;
  return { mime: file.type, data, name: file.name };
}

export function imageFiles(list: FileList | null | undefined): File[] {
  return list ? Array.from(list).filter((f) => f.type.startsWith("image/")) : [];
}

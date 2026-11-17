import {
  createContext,
  createElement,
  useCallback,
  useContext,
  type ReactNode,
} from "react";
export interface CopyReceipt {
  readonly id: string;
  readonly label: string;
  readonly status: "copied" | "failed";
}
const Receipt = createContext<((receipt: CopyReceipt) => void) | null>(null);
export function ClipboardReceiptProvider({
  onReceipt,
  children,
}: {
  onReceipt(receipt: CopyReceipt): void;
  children: ReactNode;
}) {
  return createElement(Receipt, { value: onReceipt }, children);
}
export async function copyText(text: string): Promise<boolean> {
  if (!text || typeof navigator === "undefined" || !navigator.clipboard)
    return false;
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}
export function useCopy(
  material: string,
  label: string,
  write: (material: string) => Promise<boolean> = copyText,
): () => Promise<boolean> {
  const publish = useContext(Receipt);
  return useCallback(async () => {
    const id = crypto.randomUUID();
    const accepted = await write(material);
    publish?.(
      Object.freeze({ id, label, status: accepted ? "copied" : "failed" }),
    );
    return accepted;
  }, [material, label, write, publish]);
}

import { useCallback, useLayoutEffect, useRef, useState } from "react";

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

type CopyMaterial = (material: string) => Promise<boolean>;

interface CopyFeedbackOwnership {
  material: string;
  lease: object;
  mounted: boolean;
  resetTimer: ReturnType<typeof setTimeout> | undefined;
}

function clearResetTimer(owner: CopyFeedbackOwnership): void {
  if (owner.resetTimer === undefined) return;
  clearTimeout(owner.resetTimer);
  owner.resetTimer = undefined;
}

export function useCopyFeedback(
  material: string,
  resetAfterMs = 1500,
  copyMaterial: CopyMaterial = copyText,
): { copied: boolean; copy: () => Promise<boolean> } {
  const ownerRef = useRef<CopyFeedbackOwnership>({
    material,
    lease: {},
    mounted: true,
    resetTimer: undefined,
  });
  const [accepted, setAccepted] = useState<string | null>(null);

  useLayoutEffect(() => {
    const owner = ownerRef.current;
    if (owner.material === material) return;
    owner.material = material;
    owner.lease = {};
    clearResetTimer(owner);
    setAccepted(null);
  }, [material]);

  useLayoutEffect(() => {
    const owner = ownerRef.current;
    owner.mounted = true;
    return () => {
      owner.mounted = false;
      owner.lease = {};
      clearResetTimer(owner);
    };
  }, []);

  const copy = useCallback(async (): Promise<boolean> => {
    const owner = ownerRef.current;
    const lease = (owner.lease = {});
    clearResetTimer(owner);
    setAccepted(null);

    const accepted = await copyMaterial(material);
    if (
      !accepted ||
      !owner.mounted ||
      owner.material !== material ||
      owner.lease !== lease
    ) {
      return false;
    }

    setAccepted(material);
    owner.resetTimer = setTimeout(() => {
      owner.resetTimer = undefined;
      if (
        !owner.mounted ||
        owner.material !== material ||
        owner.lease !== lease
      )
        return;
      setAccepted(null);
    }, resetAfterMs);
    return true;
  }, [copyMaterial, material, resetAfterMs]);

  return {
    copied: accepted === material,
    copy,
  };
}

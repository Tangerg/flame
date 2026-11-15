import { useCallback, useLayoutEffect, useRef, useState } from "react";
import { useContentVisible } from "./content-visibility";

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
interface CopyScope {
  active: boolean;
  attempt: object | null;
}

export function useCopyAttempt(source: unknown): () => (() => boolean) | null {
  const visible = useContentVisible();
  const [binding, setBinding] = useState<CopyScope | null>(null);
  useLayoutEffect(() => {
    // Effect reconnection retains memoized values; a new setup must own a fresh scope.
    const owned: CopyScope = { active: visible, attempt: null };
    setBinding(owned);
    return () => {
      owned.active = false;
      owned.attempt = null;
    };
  }, [source, visible]);
  return useCallback(() => {
    if (!binding?.active) return null;
    const attempt = {};
    binding.attempt = attempt;
    return () => binding.active && binding.attempt === attempt;
  }, [binding]);
}

export function useCopyFeedback(
  material: string,
  resetAfterMs = 1500,
  copyMaterial: CopyMaterial = copyText,
): { copied: boolean; copy: () => Promise<boolean | undefined> } {
  const begin = useCopyAttempt(material);
  const visible = useContentVisible();
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const [accepted, setAccepted] = useState<string | null>(null);
  const clear = useCallback(() => {
    clearTimeout(timer.current);
    timer.current = undefined;
  }, []);
  useLayoutEffect(() => {
    setAccepted(null);
    return clear;
  }, [material, visible, clear]);

  const copy = useCallback(async (): Promise<boolean | undefined> => {
    const current = begin();
    if (!current) return undefined;
    clear();
    setAccepted(null);
    const copied = await copyMaterial(material);
    if (!current()) return undefined;
    if (!copied) return false;
    setAccepted(material);
    timer.current = setTimeout(() => {
      timer.current = undefined;
      if (current()) setAccepted(null);
    }, resetAfterMs);
    return true;
  }, [begin, clear, copyMaterial, material, resetAfterMs]);
  return { copied: accepted === material, copy };
}

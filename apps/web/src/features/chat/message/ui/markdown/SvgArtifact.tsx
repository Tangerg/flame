import { ButtonPrimitive, useContentVisible } from "@flame/ui";
import * as stylex from "@stylexjs/stylex";
import {
  useCallback,
  useEffect,
  useEffectEvent,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { CodeArtifact } from "@/ui/index";
import { useT } from "@/lib/i18n/index";
import { ImagePreviewGallery } from "@/features/chat/message/ui/ImagePreviewGallery";
const sa = stylex.create({
  trigger: {
    display: "block",
    width: "100%",
    borderWidth: 0,
    backgroundColor: "transparent",
    padding: 0,
    cursor: "zoom-in",
  },
  preview: {
    display: "block",
    maxHeight: "calc(var(--spacing) * 96)",
    width: "100%",
    objectFit: "contain",
  },
});

export function SvgArtifact({ code, lang }: { code: string; lang: string }) {
  const t = useT();
  const visible = useContentVisible();
  const src = useMemo(() => `data:image/svg+xml;charset=utf-8,${encodeURIComponent(code)}`, [code]);
  const label = t("message.svg.generatedAlt");
  const [previewSrc, setPreviewSrc] = useState<string>();
  const acceptDecode = useEffectEvent((requested: string, decoded: boolean) => {
    if (requested === src) setPreviewSrc(decoded ? requested : undefined);
  });
  useEffect(() => {
    const image = new Image();
    image.src = src;
    let cancelled = false;
    const settle = (decoded: boolean) => {
      if (!cancelled) acceptDecode(src, decoded);
    };
    void image.decode().then(
      () => settle(true),
      () => settle(false),
    );
    return () => {
      cancelled = true;
    };
  }, [src]);

  const entry = useRef<HTMLButtonElement | HTMLDivElement>(null);
  const retiredEntry = useRef<HTMLElement>(null);
  const entryRef = useCallback((node: HTMLButtonElement | HTMLDivElement | null) => {
    if (!node) return;
    entry.current = node;
    if (retiredEntry.current === node) retiredEntry.current = null;
    return () => {
      if (node.ownerDocument.activeElement === node) retiredEntry.current = node;
      if (entry.current === node) entry.current = null;
    };
  }, []);
  useLayoutEffect(() => {
    const retiring = retiredEntry.current;
    retiredEntry.current = null;
    const target = entry.current;
    if (
      visible &&
      retiring?.isConnected === false &&
      target?.isConnected &&
      target.ownerDocument.activeElement === target.ownerDocument.body
    ) {
      target.focus({ preventScroll: true });
    }
  }, [previewSrc, visible]);

  return (
    <ImagePreviewGallery
      item={previewSrc ? { src: previewSrc, alt: label } : undefined}
      renderEntry={(preview) => (
        <CodeArtifact lang={lang} code={code} preview={preview} sourceBodyRef={entryRef} />
      )}
      finalFocus={previewSrc ? undefined : entry}
      trigger={(previewProps) => (
        <ButtonPrimitive
          ref={entryRef}
          type="button"
          aria-label={label}
          {...previewProps}
          {...stylex.props(sa.trigger)}
        >
          <img src={previewSrc} alt={label} {...stylex.props(sa.preview)} />
        </ButtonPrimitive>
      )}
    />
  );
}

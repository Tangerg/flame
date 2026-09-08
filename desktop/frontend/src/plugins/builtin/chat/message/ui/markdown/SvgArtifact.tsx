import * as stylex from "@stylexjs/stylex";
import { useMemo } from "react";
import { ShikiCodeBlock } from "@/ui";
import { useT } from "@/lib/i18n";

const sa = stylex.create({
  preview: {
    display: "block",
    maxHeight: "calc(var(--spacing) * 96)",
    width: "100%",
    objectFit: "contain",
  },
});

export function SvgArtifact({ code, lang }: { code: string; lang: string }) {
  const t = useT();
  const src = useMemo(() => `data:image/svg+xml;charset=utf-8,${encodeURIComponent(code)}`, [code]);
  const label = t("message.svg.generatedAlt");

  return (
    <ShikiCodeBlock
      lang={lang}
      code={code}
      previewLabel={label}
      preview={<img src={src} alt={label} {...stylex.props(sa.preview)} />}
    />
  );
}

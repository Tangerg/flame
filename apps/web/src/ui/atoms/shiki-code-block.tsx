import * as stylex from "@stylexjs/stylex";
import { type ReactNode } from "react";
import { CodeBody, CodeCaption, CodeFrame } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { cn } from "@/lib/classNames";
import { space } from "@flame/ui/tokens.stylex";
import { artifactFrame } from "@/ui/atoms/artifact-frame";
import { reveal } from "@/ui/atoms/reveal";
import { useCodeActionLabels } from "@/ui/atoms/codeActionLabels";
interface Props {
  lang: string;
  code: string;
  preview?: ReactNode;
  previewLabel?: string;
  scrollLabel?: string;
  copyable?: boolean;
}

const styles = stylex.create({
  embedded: { marginBlock: 0 },
  previewBody: {
    display: "grid",
    minHeight: 0,
    maxHeight: "calc(15lh + 24px)",
    placeItems: "center",
    overflow: "auto",
    padding: space.s3,
  },
  compact: { maxHeight: "calc(var(--spacing) * 60)" },
});

export function ShikiCodeBlock({
  lang,
  code,
  preview,
  previewLabel,
  scrollLabel,
  copyable = true,
}: Props) {
  const labels = useCodeActionLabels();
  const t = useT();
  const isPreview = preview !== undefined;
  return (
    <CodeFrame
      dir="ltr"
      data-slot="code-block"
      data-focus-unit=""
      data-variant={isPreview ? "preview" : "code"}
      data-markdown-copy="code-block"
      data-markdown-copy-text={code}
      styles={[artifactFrame.frame, scrollLabel !== undefined && styles.embedded]}
      className={cn("shiki-block", isPreview && stylex.props(reveal.host).className)}
    >
      <CodeCaption
        data-markdown-copy="exclude"
        language={lang}
        code={code}
        labels={labels}
        wrappable={!isPreview}
        copyable={copyable}
        copyReveal={
          isPreview
            ? { "data-reveal": "hover", className: stylex.props(reveal.shown).className ?? "" }
            : undefined
        }
      />
      {isPreview ? (
        <div
          data-slot="shiki-preview-body"
          data-focus-inset=""
          {...stylex.props(styles.previewBody)}
          role="region"
          aria-label={previewLabel}
          // oxlint-disable-next-line jsx-a11y/no-noninteractive-tabindex
          tabIndex={0}
        >
          {preview}
        </div>
      ) : (
        <CodeBody
          label={scrollLabel ?? t("message.code.content")}
          language={lang}
          styles={scrollLabel !== undefined && styles.compact}
        >
          {code}
        </CodeBody>
      )}
    </CodeFrame>
  );
}

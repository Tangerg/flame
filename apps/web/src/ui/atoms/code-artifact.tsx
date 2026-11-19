import * as stylex from "@stylexjs/stylex";
import { type ReactNode, type Ref } from "react";
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
  scrollLabel?: string;
  copyable?: boolean;
  ref?: Ref<HTMLDivElement>;
  sourceBodyRef?: Ref<HTMLDivElement>;
}

const previewButtonFocus = 'button[data-message-image-preview-trigger="true"]:focus-visible';
const previewFocus = `:has(> ${previewButtonFocus})`;

const styles = stylex.create({
  embedded: { marginBlock: 0 },
  previewBody: {
    display: "grid",
    minHeight: 0,
    maxHeight: "calc(15lh + 24px)",
    placeItems: "center",
    overflow: "auto",
    // Clip the scrolling image before it reaches the inset focus ring.
    borderWidth: "var(--focus-inset-edge)",
    borderStyle: "solid",
    borderColor: "transparent",
    padding: `calc(${space.s3} - var(--focus-inset-edge))`,
    [previewFocus]: {
      outlineStyle: "solid",
      outlineWidth: "1.5px",
      outlineColor: {
        default: "var(--color-focus-ring)",
        "@media (forced-colors: active)": "Highlight",
      },
      outlineOffset: "calc(-1 * var(--focus-inset-edge))",
    },
    [`${previewFocus} > ${previewButtonFocus}`]: { outline: "none" },
  },
  compact: { maxHeight: "calc(var(--spacing) * 60)" },
});

export function CodeArtifact({
  lang,
  code,
  preview,
  scrollLabel,
  copyable = true,
  ref,
  sourceBodyRef,
}: Props) {
  const labels = useCodeActionLabels();
  const t = useT();
  const isPreview = preview !== undefined;
  return (
    <CodeFrame
      ref={ref}
      bounded
      dir="ltr"
      data-slot="code-block"
      data-focus-unit=""
      data-variant={isPreview ? "preview" : "code"}
      data-markdown-copy="code-block"
      data-markdown-copy-text={code}
      styles={[artifactFrame.frame, scrollLabel !== undefined && styles.embedded]}
      className={cn("code-artifact", isPreview && stylex.props(reveal.host).className)}
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
          data-slot="code-preview-body"
          data-focus-inset=""
          {...stylex.props(styles.previewBody)}
        >
          {preview}
        </div>
      ) : (
        <CodeBody
          ref={sourceBodyRef}
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

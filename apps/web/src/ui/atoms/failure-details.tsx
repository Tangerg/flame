import * as stylex from "@stylexjs/stylex";
import { useState } from "react";
import { useCopyFeedback } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { space } from "@flame/ui/tokens.stylex";
import { TextButton } from "@/ui/atoms/text-button";
import { Well } from "@/ui/atoms/well";
const styles = stylex.create({
  root: { display: "flex", minWidth: 0, flexDirection: "column", gap: space.s2 },
  actions: { display: "flex", flexWrap: "wrap", gap: space.s3 },
});

export function FailureDetails({ diagnostics }: { diagnostics: string }) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const { copied, copy } = useCopyFeedback(diagnostics);
  return (
    <div {...stylex.props(styles.root)}>
      <div {...stylex.props(styles.actions)}>
        <TextButton tone="muted" aria-expanded={open} onClick={() => setOpen((value) => !value)}>
          {t("plugins.renderFailed.details")}
        </TextButton>
        <TextButton tone="muted" onClick={() => void copy()}>
          {copied ? t("startup.failed.copied") : t("startup.failed.copy")}
        </TextButton>
      </div>
      {open && (
        <Well wrap="anywhere" cap="md">
          {diagnostics}
        </Well>
      )}
    </div>
  );
}

import { useCallback, useLayoutEffect, useRef, useState } from "react";
import * as stylex from "@stylexjs/stylex";
import { copyText } from "@flame/ui";
import { space } from "@flame/ui/tokens.stylex";
import { useComposer } from "@/features/composer/public/composer";
import { type ComposerPersistenceState } from "@/features/composer/application/state";
import { useT } from "@/lib/i18n/index";
import { SystemMessage, TextButton } from "@/ui/index";

const styles = stylex.create({
  actions: { display: "flex", flexWrap: "wrap", gap: space.s3, marginTop: space.s1 },
});

interface CopyOwner {
  active: boolean;
  attempt: object | null;
}

export function ComposerPersistenceNotice() {
  const composer = useComposer();
  const persistence = composer.usePersistenceState();
  const t = useT();
  const copyOwner = useRef<CopyOwner>({ active: false, attempt: null });
  const returnFromRetry = useRef<(() => void) | null>(null);
  const [copyResult, setCopyResult] = useState<{
    source: ComposerPersistenceState;
    accepted: boolean | null;
  }>({ source: persistence, accepted: null });
  if (copyResult.source !== persistence) {
    setCopyResult({ source: persistence, accepted: null });
  }

  useLayoutEffect(() => {
    const owner: CopyOwner = { active: true, attempt: null };
    copyOwner.current = owner;
    return () => {
      owner.active = false;
    };
  }, [persistence]);

  const bindRetry = useCallback((button: HTMLButtonElement | null) => {
    if (!button) return;
    return () => {
      if (button.ownerDocument.activeElement === button) returnFromRetry.current?.();
    };
  }, []);

  if (persistence.status === "saved") return null;

  const copyDrafts = async () => {
    const owner = copyOwner.current;
    const attempt = (owner.attempt = {});
    let accepted = false;
    try {
      accepted = await copyText(composer.getRecoveryDrafts());
    } catch {
      accepted = false;
    }
    if (owner.active && owner.attempt === attempt) {
      setCopyResult({ source: persistence, accepted });
    }
  };
  const copied = copyResult.source === persistence ? copyResult.accepted : null;
  const message =
    persistence.status === "recovery-required"
      ? "composer.persistence.recoveryRequired"
      : persistence.operation === "read"
        ? "composer.persistence.readFailed"
        : "composer.persistence.saveFailed";

  return (
    <SystemMessage variant="error" icon="alert">
      {t(message)}
      <div {...stylex.props(styles.actions)}>
        {persistence.status !== "recovery-required" && (
          <TextButton
            ref={bindRetry}
            tone="accent"
            onMouseDown={(event) => event.preventDefault()}
            onFocus={() => {
              returnFromRetry.current = composer.captureFocus();
            }}
            onClick={composer.retryPersistence}
          >
            {t("common.retry")}
          </TextButton>
        )}
        <TextButton tone="accent" onClick={() => void copyDrafts()}>
          {t(copied ? "composer.persistence.copied" : "composer.persistence.copy")}
        </TextButton>
      </div>
      {copied === false && <p>{t("composer.persistence.copyFailed")}</p>}
    </SystemMessage>
  );
}

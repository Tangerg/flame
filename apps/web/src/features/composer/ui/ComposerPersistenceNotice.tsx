import { useLayoutEffect, useRef, useState } from "react";
import * as stylex from "@stylexjs/stylex";
import { copyText, useCopyAttempt, useContentVisible } from "@flame/ui";
import { space } from "@flame/ui/tokens.stylex";
import { useComposer } from "@/features/composer/public/composer";
import { type ComposerAccess } from "@/features/composer/public/composer";
import { type ComposerPersistenceState } from "@/features/composer/application/state";
import { useT } from "@/lib/i18n/index";
import { SystemMessage, TextButton } from "@/ui/index";

const styles = stylex.create({
  actions: { display: "flex", flexWrap: "wrap", gap: space.s3, marginTop: space.s1 },
});

export function ComposerPersistenceNotice() {
  return (
    <>
      <CurrentDraftNotice />
      <LegacyDraftNotice />
    </>
  );
}

function CurrentDraftNotice() {
  const composer = useComposer();
  const persistence = composer.usePersistenceState();
  const t = useT();
  const beginCopy = useCopyAttempt(persistence);
  const [copyResult, setCopyResult] = useState<{
    source: ComposerPersistenceState;
    accepted: boolean | null;
  }>({ source: persistence, accepted: null });
  if (copyResult.source !== persistence) {
    setCopyResult({ source: persistence, accepted: null });
  }

  if (persistence.status === "saved" || (persistence.status === "pending" && !persistence.retrying))
    return null;

  const copyDrafts = async () => {
    const current = beginCopy();
    if (!current) return;
    const text = composer.getRecoveryDrafts();
    const accepted = await copyText(text);
    if (current()) setCopyResult({ source: persistence, accepted });
  };
  const copied = copyResult.source === persistence ? copyResult.accepted : null;
  const message =
    persistence.status === "loading"
      ? "composer.persistence.loading"
      : persistence.status === "pending"
        ? "composer.persistence.pending"
        : persistence.status === "recovery-required"
          ? "composer.persistence.recoveryRequired"
          : persistence.operation === "read"
            ? "composer.persistence.readFailed"
            : "composer.persistence.saveFailed";

  return (
    <SystemMessage
      variant={
        persistence.status === "loading" || persistence.status === "pending" ? "info" : "error"
      }
    >
      {t(message)}
      <div {...stylex.props(styles.actions)}>
        {(persistence.status === "failed" ||
          (persistence.status === "pending" && persistence.retrying)) && (
          <PersistenceRetry
            composer={composer}
            pending={persistence.status === "pending"}
            label={t("common.retry")}
          />
        )}
        <TextButton tone="accent" onClick={() => void copyDrafts()}>
          {t(copied ? "composer.persistence.copied" : "composer.persistence.copy")}
        </TextButton>
      </div>
      {copied === false && <p>{t("composer.persistence.copyFailed")}</p>}
    </SystemMessage>
  );
}

function LegacyDraftNotice() {
  const composer = useComposer();
  const state = composer.useLegacyDraftState();
  const t = useT();
  const beginCopy = useCopyAttempt(state);
  const [copied, setCopied] = useState<{ source: typeof state; result: boolean | null }>({
    source: state,
    result: null,
  });
  if (state.status === "none") return null;
  const copy = async () => {
    const current = beginCopy();
    if (!current) return;
    const text = composer.getLegacyDraftBackup();
    const accepted = await copyText(text);
    if (current()) setCopied({ source: state, result: accepted });
  };
  const result = copied.source === state ? copied.result : null;
  return (
    <SystemMessage variant={state.status === "failed" ? "error" : "info"}>
      {t(
        state.status === "blocked"
          ? "composer.legacy.blocked"
          : state.status === "failed"
            ? "composer.legacy.failed"
            : state.status === "importing"
              ? "composer.legacy.importing"
              : "composer.legacy.available",
      )}
      <div {...stylex.props(styles.actions)}>
        {(state.status === "available" || (state.status === "failed" && state.canImport)) && (
          <TextButton
            tone="accent"
            onMouseDown={(event) => event.preventDefault()}
            onClick={composer.importLegacyDrafts}
          >
            {t("composer.legacy.import")}
          </TextButton>
        )}
        <TextButton tone="accent" onClick={() => void copy()}>
          {t(result ? "composer.legacy.copied" : "composer.legacy.copy")}
        </TextButton>
      </div>
      {result === false && <p>{t("composer.persistence.copyFailed")}</p>}
    </SystemMessage>
  );
}

function PersistenceRetry({
  composer,
  pending,
  label,
}: {
  composer: ComposerAccess;
  pending: boolean;
  label: string;
}) {
  const button = useRef<HTMLButtonElement>(null);
  const capture = useRef<{ composer: ComposerAccess; restore(): void } | null>(null);
  const visible = useContentVisible();
  const presentation = useRef({ composer, visible });
  useLayoutEffect(() => {
    presentation.current = { composer, visible };
  });
  useLayoutEffect(
    () => () => {
      const node = button.current;
      const original = capture.current;
      capture.current = null;
      if (
        node &&
        node.ownerDocument.activeElement === node &&
        presentation.current.visible &&
        original?.composer === presentation.current.composer
      )
        original.restore();
    },
    [],
  );
  return (
    <TextButton
      ref={button}
      tone="accent"
      aria-disabled={pending || undefined}
      onMouseDown={(event) => event.preventDefault()}
      onFocus={() => {
        capture.current = { composer, restore: composer.captureFocus() };
      }}
      onClick={composer.retryPersistence}
    >
      {label}
    </TextButton>
  );
}

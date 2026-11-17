import * as stylex from "@stylexjs/stylex";
import { useCopy } from "@flame/ui";
import { space } from "@flame/ui/tokens.stylex";
import { useComposer } from "@/features/composer/public/composer";
import { type ComposerAccess } from "@/features/composer/public/composer";
import { useT } from "@/lib/i18n/index";
import { SystemMessage, TextButton } from "@/ui/index";

const styles = stylex.create({
  actions: { display: "flex", flexWrap: "wrap", gap: space.s3, marginTop: space.s1 },
});

export function ComposerPersistenceNotice() {
  const composer = useComposer();
  const persistence = composer.usePersistenceState();
  const t = useT();
  const copyDrafts = useCopy(composer.getRecoveryDrafts(), t("composer.persistence.copy"));

  if (persistence.status === "saved" || (persistence.status === "pending" && !persistence.retrying))
    return null;

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
          {t("composer.persistence.copy")}
        </TextButton>
      </div>
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
  return (
    <TextButton
      tone="accent"
      pending={pending}
      onClick={() => {
        const restore = composer.captureFocus();
        composer.retryPersistence();
        restore();
      }}
    >
      {label}
    </TextButton>
  );
}

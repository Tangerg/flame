import * as stylex from "@stylexjs/stylex";
import { useEffect, useState, type RefObject } from "react";
import { Button, FormActions, Metadata, ModalDialog } from "@flame/ui";
import type { SessionForkPreview } from "@flame/runtime-client/wire";
import { useSessionWorktreeFork } from "@/features/agent/public/session";
import { useT } from "@/lib/i18n";
import { rpcErrorText } from "@/lib/rpcErrors";

const styles = stylex.create({
  patch: { maxHeight: "40vh", overflow: "auto", marginBlock: "0.75rem" },
  path: { overflowWrap: "anywhere" },
});

export function SessionWorktreeForkDialog({
  sessionId,
  close,
  finalFocus,
}: {
  sessionId: string;
  close(): void;
  finalFocus: RefObject<HTMLElement | null>;
}) {
  const t = useT();
  const { preview, confirm } = useSessionWorktreeFork();
  const [value, setValue] = useState<SessionForkPreview>();
  const [failure, setFailure] = useState<unknown>();
  const [submitting, setSubmitting] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    void preview(sessionId, controller.signal).then(
      (next) => {
        if (!controller.signal.aborted) setValue(next);
      },
      (error: unknown) => {
        if (!controller.signal.aborted) setFailure(error);
      },
    );
    return () => controller.abort();
  }, [preview, sessionId]);
  const apply = async () => {
    if (!value || submitting) return;
    setSubmitting(true);
    setFailure(undefined);
    try {
      await confirm(sessionId, value.worktree);
      close();
    } catch (error) {
      setFailure(error);
    } finally {
      setSubmitting(false);
    }
  };
  const title = t("session.forkWorktree.title");
  return (
    <ModalDialog
      open
      title={title}
      closeLabel={t("common.close")}
      size="lg"
      finalFocus={finalFocus}
      onOpenChange={(open) => {
        if (!open && !submitting) close();
      }}
    >
      {failure !== undefined && (
        <p role="alert">{rpcErrorText(failure) ?? t("session.forkWorktree.error")}</p>
      )}
      {value ? (
        <>
          <Metadata>
            {t("session.forkWorktree.history", { messages: value.messages, runs: value.runs })}
          </Metadata>
          {value.fromRunId && (
            <p>
              <code>{value.fromRunId}</code>
            </p>
          )}
          <p {...stylex.props(styles.path)}>
            {t("session.forkWorktree.destination")} <code>{value.destination.path}</code>
          </p>
          <p>
            <code>{value.head}</code>
          </p>
          {value.patch ? (
            <pre {...stylex.props(styles.patch)}>{value.patch}</pre>
          ) : (
            <p>{t("session.forkWorktree.noChanges")}</p>
          )}
          <FormActions label={title}>
            <Button
              type="button"
              variant="primary"
              disabled={submitting}
              onClick={() => void apply()}
            >
              {title}
            </Button>
          </FormActions>
        </>
      ) : (
        failure === undefined && <p role="status">{t("packages.module.loading")}</p>
      )}
    </ModalDialog>
  );
}

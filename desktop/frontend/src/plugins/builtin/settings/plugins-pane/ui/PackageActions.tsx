import { useState } from "react";
import type {
  PluginAction,
  PluginInstallation,
  RenamePluginSessionRequest,
  Session,
} from "@flame/runtime-contract/wire";
import { useT } from "@/lib/i18n";
import { InputDialog, PillButton, SystemMessage } from "@/ui";
import { invalidateAgentSessions, useActiveSession } from "@/plugins/builtin/agent/public/session";
import { useAsyncFeedback } from "../../kit";
import type { packageOperations } from "../application/packages";

type Operations = ReturnType<typeof packageOperations.get>;
interface Edit {
  action: PluginAction;
  request: RenamePluginSessionRequest;
  operations: Operations;
}

export function PackageActions({
  installation,
  operations,
}: {
  installation: PluginInstallation;
  operations: Operations;
}) {
  const t = useT();
  const session = useActiveSession();
  const [edit, setEdit] = useState<Edit>();
  const [receipt, setReceipt] = useState<{ operations: Operations; session: Session }>();
  return (
    <>
      {!session && <SystemMessage>{t("packages.action.selectSession")}</SystemMessage>}
      {receipt && receipt.operations === operations && !operations.signal.aborted && (
        <SystemMessage variant="success">
          {t("packages.action.renamed", {
            sessionId: receipt.session.id,
            title: receipt.session.title || t("session.untitled"),
          })}
        </SystemMessage>
      )}
      {installation.selected.actions.map((action) => (
        <PillButton
          key={action.id}
          size="sm"
          disabled={!session || operations.signal.aborted}
          onClick={() => {
            if (!session || operations.signal.aborted) return;
            setReceipt(undefined);
            setEdit({
              action,
              operations,
              request: {
                installationId: installation.id,
                digest: installation.selected.digest,
                actionId: action.id,
                update: {
                  sessionId: session.id,
                  expectedRevision: session.revision,
                  title: session.title,
                },
              },
            });
          }}
        >
          {action.title} · {t("session.action.rename")}
        </PillButton>
      ))}
      {edit && edit.operations === operations && !operations.signal.aborted && (
        <RenameForm
          key={`${edit.request.digest}:${edit.request.actionId}:${edit.request.update.sessionId}`}
          edit={edit}
          onSaved={(session) => {
            setReceipt({ operations: edit.operations, session });
            setEdit((current) => (current === edit ? undefined : current));
          }}
          onClose={() => setEdit((current) => (current === edit ? undefined : current))}
        />
      )}
    </>
  );
}

function RenameForm({
  edit,
  onClose,
  onSaved,
}: {
  edit: Edit;
  onClose(): void;
  onSaved(session: Session): void;
}) {
  const t = useT();
  const [title, setTitle] = useState(edit.request.update.title);
  const { feedback, run } = useAsyncFeedback(edit.operations.signal);
  const save = () =>
    run(async () => {
      edit.operations.signal.throwIfAborted();
      const saved = await edit.operations.renameSession({
        ...edit.request,
        update: { ...edit.request.update, title },
      });
      if (!edit.operations.signal.aborted) {
        void invalidateAgentSessions();
        onSaved(saved);
      }
      return { ok: true };
    }, t("session.action.rename"));
  return (
    <InputDialog
      title={`${edit.action.title} · ${t("session.action.rename")}`}
      description={
        <>
          {t("packages.action.target", {
            sessionId: edit.request.update.sessionId,
            revision: edit.request.update.expectedRevision,
          })}
          {feedback.state === "error" && <span role="alert"> · {feedback.reason}</span>}
        </>
      }
      label={t("session.row.titleLabel")}
      value={title}
      busy={feedback.state === "busy"}
      confirmLabel={t("session.action.rename")}
      cancelLabel={t("common.cancel")}
      onChange={setTitle}
      onConfirm={() => void save()}
      onClose={onClose}
    />
  );
}

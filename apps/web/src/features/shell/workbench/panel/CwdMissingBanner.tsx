import * as stylex from "@stylexjs/stylex";
import { useRef, useState } from "react";
import { SystemMessage } from "@/ui/index";
import { isImeKey, TextField, vocab } from "@flame/ui";
import { useActiveSession } from "@/features/agent/public/session";
import { useRelocateSession } from "@/features/agent/public/session";
import { BannerAction } from "@/features/shell/workbench/panel/BannerAction";
import { useT } from "@/lib/i18n/index";
import { useRuntimeCapability } from "@/features/connection/public/capabilities";
import { shellStyles as sh } from "@/features/shell/workbench/shellStyles";
import { color, face, space, type as typeStep, weight } from "@flame/ui/tokens.stylex";
const cw = stylex.create({
  title: { marginBottom: space.s0_5, color: color.warning, fontWeight: weight.semibold },
  body: { color: color.fgSoft, overflowWrap: "break-word" },
  form: { marginTop: space.s2 },
  field: { width: "calc(var(--spacing) * 72)", maxWidth: "100%" },
});

export function CwdMissingBanner() {
  const t = useT();
  const session = useActiveSession();
  const relocateEnabled = useRuntimeCapability("relocate");
  const relocate = useRelocateSession();
  const [edit, setEdit] = useState<{
    target: Readonly<{ id: string; revision: number; relocate: typeof relocate }>;
    path: string;
  } | null>(null);
  const [busy, setBusy] = useState(false);
  const submitting = useRef(false);

  if (session?.workspace.availability !== "missing") return null;
  const editing = edit?.target.id === session.id ? edit : null;

  const cancelEdit = () =>
    setEdit((current) => (current?.target === editing?.target ? null : current));
  const submit = async (): Promise<void> => {
    const submitted = editing;
    const next = submitted?.path.trim();
    if (!submitted || !next || submitting.current) return;
    submitting.current = true;
    setBusy(true);
    const ok = await submitted.target.relocate(
      submitted.target.id,
      submitted.target.revision,
      next,
    );
    submitting.current = false;
    setBusy(false);
    if (ok) {
      setEdit((current) => (current === submitted ? null : current));
    }
  };

  return (
    <SystemMessage variant="warning" shape="form" className={stylex.props(sh.banner).className}>
      <div {...stylex.props(vocab.min)}>
        <div {...stylex.props(cw.title, typeStep.uiMd)}>{t("cwdMissing.title")}</div>
        <div {...stylex.props(cw.body, typeStep.uiMd)}>
          <code {...stylex.props(typeStep.uiMd, face.mono)}>{session.workspace.path}</code> ·{" "}
          {t("cwdMissing.body")}
        </div>
        {relocateEnabled && (
          <div {...stylex.props(cw.form)}>
            {editing ? (
              <div {...stylex.props(vocab.lineTight)}>
                <TextField
                  font="mono"
                  type="text"
                  size="sm"
                  value={editing.path}
                  onChange={(e) => {
                    const path = e.target.value;
                    setEdit((current) =>
                      current?.target === editing.target ? { ...current, path } : current,
                    );
                  }}
                  onKeyDown={(e) => {
                    if (isImeKey(e.nativeEvent)) return;
                    if (e.key === "Enter") void submit();
                    if (e.key === "Escape") cancelEdit();
                  }}
                  placeholder={t("cwdMissing.placeholder")}
                  aria-label={t("cwdMissing.placeholder")}
                  pending={busy}
                  spellCheck={false}
                  // oxlint-disable-next-line jsx-a11y/no-autofocus
                  autoFocus
                  className={stylex.props(cw.field).className}
                />
                <BannerAction
                  label={t("cwdMissing.action.apply")}
                  onClick={() => void submit()}
                  pending={busy}
                  primary
                  tone="warning"
                />
                <BannerAction
                  label={t("cwdMissing.action.cancel")}
                  onClick={cancelEdit}
                  pending={busy}
                />
              </div>
            ) : (
              <BannerAction
                label={t("cwdMissing.action.relocate")}
                onClick={() => {
                  setEdit({
                    target: Object.freeze({ id: session.id, revision: session.revision, relocate }),
                    path: session.workspace.path,
                  });
                }}
                primary
                tone="warning"
              />
            )}
          </div>
        )}
      </div>
    </SystemMessage>
  );
}

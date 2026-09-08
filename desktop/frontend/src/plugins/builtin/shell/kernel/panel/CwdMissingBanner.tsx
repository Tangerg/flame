import * as stylex from "@stylexjs/stylex";
import { useRef, useState } from "react";
import { SystemMessage, TextField, vocab } from "@/ui";
import { useActiveSession, useRelocateSession } from "@/plugins/builtin/agent/public/session";
import { BannerAction } from "./BannerAction";
import { useT } from "@/lib/i18n";
import { useRuntimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import { shellStyles as sh } from "../shellStyles";
import { color, face, space, type as typeStep, weight } from "@/styles/tokens.stylex";

const cw = stylex.create({
  title: { marginBottom: space.s0_5, color: color.warning, fontWeight: weight.semibold },
  body: { color: color.fgSoft, overflowWrap: "break-word" },
  form: { marginTop: space.s2 },
  // Wide enough for a path and no wider: the banner sits inside the reading column.
  field: { width: "calc(var(--spacing) * 72)", maxWidth: "100%" },
});

export function CwdMissingBanner() {
  const t = useT();
  const session = useActiveSession();
  const relocateEnabled = useRuntimeCapability("relocate");
  const relocate = useRelocateSession();
  const [editing, setEditing] = useState(false);
  const [path, setPath] = useState("");
  const [busy, setBusy] = useState(false);
  const submitting = useRef(false);

  if (session?.workspace.availability !== "missing") return null;

  const submit = async (): Promise<void> => {
    const next = path.trim();
    if (!next || submitting.current) return;
    submitting.current = true;
    setBusy(true);
    const ok = await relocate(session.id, session.revision, next);
    submitting.current = false;
    setBusy(false);
    if (ok) {
      setEditing(false);
      setPath("");
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
                  type="text"
                  size="sm"
                  value={path}
                  onChange={(e) => setPath(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.nativeEvent.isComposing) return;
                    if (e.key === "Enter") void submit();
                    if (e.key === "Escape") setEditing(false);
                  }}
                  placeholder={t("cwdMissing.placeholder")}
                  aria-label={t("cwdMissing.placeholder")}
                  disabled={busy}
                  spellCheck={false}
                  // oxlint-disable-next-line jsx-a11y/no-autofocus
                  autoFocus
                  className={stylex.props(cw.field).className}
                />
                {/* In flight, the label STAYS and the control shuts — the same way the
                    approval card reports a decision it is waiting on. Swapping the label for
                    "…" made the button's accessible name an ellipsis, left it untranslated in
                    eight locales, and collapsed its width mid-click so Cancel jumped left. */}
                <BannerAction
                  label={t("cwdMissing.action.apply")}
                  onClick={() => void submit()}
                  disabled={busy}
                  primary
                  tone="warning"
                />
                <BannerAction
                  label={t("cwdMissing.action.cancel")}
                  onClick={() => setEditing(false)}
                  disabled={busy}
                />
              </div>
            ) : (
              <BannerAction
                label={t("cwdMissing.action.relocate")}
                onClick={() => {
                  setPath(session.workspace.path);
                  setEditing(true);
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

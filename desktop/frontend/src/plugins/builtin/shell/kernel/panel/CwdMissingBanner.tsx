import { useRef, useState } from "react";
import { SystemMessage, TextField } from "@/ui";
import { useActiveSession, useRelocateSession } from "@/plugins/builtin/agent/public/session";
import { BannerAction } from "./BannerAction";
import { useT } from "@/lib/i18n";
import { useRuntimeCapability } from "@/plugins/builtin/runtime/public/capabilities";

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
    <SystemMessage variant="warning" className="my-2.5 items-start px-3 py-2.5">
      <div className="min-w-0">
        <div className="mb-0.5 text-ui-md font-semibold text-warning">{t("cwdMissing.title")}</div>
        <div className="text-ui-md text-fg-soft break-words">
          <code className="font-mono text-ui-md">{session.workspace.path}</code> ·{" "}
          {t("cwdMissing.body")}
        </div>
        {relocateEnabled && (
          <div className="mt-2">
            {editing ? (
              <div className="flex items-center gap-1.5">
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
                  className="w-72 max-w-full"
                />
                <BannerAction
                  label={busy ? "…" : t("cwdMissing.action.apply")}
                  onClick={() => void submit()}
                  primary
                  tone="warning"
                />
                <BannerAction
                  label={t("cwdMissing.action.cancel")}
                  onClick={() => setEditing(false)}
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

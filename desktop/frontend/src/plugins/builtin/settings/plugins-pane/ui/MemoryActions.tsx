import { useState } from "react";
import * as stylex from "@stylexjs/stylex";
import { useT } from "@/lib/i18n";
import { PillButton, DropdownMenu, SelectTrigger, SystemMessage, TextArea, vocab } from "@/ui";
import { space } from "@/styles/tokens.stylex";
import {
  useAgentMemory,
  addAgentMemory,
  reviewAgentMemory,
  updateAgentMemoryContent,
  setAgentMemoryPinned,
  deleteAgentMemory,
  type AgentMemoryEntry,
  type AgentMemoryQuery,
} from "@/plugins/builtin/workspace/public/memory";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useAsyncFeedback } from "../../kit";

const styles = stylex.create({
  root: { paddingInline: space.s3, paddingBottom: space.s3, flexShrink: 0 },
  controls: { display: "flex", flexWrap: "wrap", gap: space.s2 },
  editor: { paddingTop: space.s2 },
  picker: { maxWidth: "100%" },
});
type Edit = { type: "add" } | { type: "update"; item: AgentMemoryEntry };
export function MemoryActions({
  scope,
  cwd,
  signal,
  onSaved,
}: AgentMemoryQuery & { signal: AbortSignal; onSaved(): void }) {
  const t = useT();
  const { data: items = [], isLoading, error, refetch } = useAgentMemory(true, scope, cwd);
  const [selectedId, setSelectedId] = useState<string>();
  const [edit, setEdit] = useState<Edit>();
  const [draft, setDraft] = useState("");
  const { feedback, run } = useAsyncFeedback(signal);
  const busy = feedback.state === "busy";
  const selected = items.find((item) => item.id === selectedId);
  const execute = (operation: () => Promise<unknown>) =>
    void run(
      async () => {
        signal.throwIfAborted();
        await operation();
        if (!signal.aborted) {
          setEdit(undefined);
          setDraft("");
          onSaved();
        }
        return { ok: true };
      },
      t("agentMemory.error"),
      wasGenerationRetired,
    );
  return (
    <div {...stylex.props(styles.root)}>
      <div {...stylex.props(styles.controls)}>
        <PillButton
          size="sm"
          pending={busy}
          onClick={() => {
            setEdit({ type: "add" });
            setDraft("");
          }}
        >
          {t("agentMemory.add")}
        </PillButton>
        <DropdownMenu.Root>
          <DropdownMenu.Trigger
            render={
              <SelectTrigger
                aria-label={t("agentMemory.select")}
                label={
                  selected
                    ? `${selected.id} · ${selected.status} · ${selected.content}`
                    : t("agentMemory.select")
                }
                pending={busy}
                disabled={isLoading || Boolean(error) || items.length === 0}
                className={stylex.props(styles.picker).className}
              />
            }
          />
          <DropdownMenu.Content align="start">
            {items.map((item) => (
              <DropdownMenu.Item
                key={item.id}
                onClick={() => {
                  setSelectedId(item.id);
                  setEdit(undefined);
                }}
                layout="pickPlain"
              >
                <span {...stylex.props(vocab.truncate)}>
                  {item.id} · {item.status} · {item.content}
                </span>
              </DropdownMenu.Item>
            ))}
          </DropdownMenu.Content>
        </DropdownMenu.Root>
        {selected && !error && !isLoading && (
          <>
            {selected.status === "pending" ? (
              <>
                <PillButton
                  size="sm"
                  pending={busy}
                  onClick={() => execute(() => reviewAgentMemory(selected.id, "approve"))}
                >
                  {t("agentMemory.approve")}
                </PillButton>
                <PillButton
                  size="sm"
                  pending={busy}
                  onClick={() => execute(() => reviewAgentMemory(selected.id, "reject"))}
                >
                  {t("agentMemory.reject")}
                </PillButton>
              </>
            ) : (
              <>
                <PillButton
                  size="sm"
                  pending={busy}
                  onClick={() => {
                    setEdit({ type: "update", item: selected });
                    setDraft(selected.content);
                  }}
                >
                  {t("agentMemory.edit")}
                </PillButton>
                <PillButton
                  size="sm"
                  pending={busy}
                  onClick={() => execute(() => setAgentMemoryPinned(selected.id, !selected.pinned))}
                >
                  {t(selected.pinned ? "agentMemory.unpin" : "agentMemory.pin")}
                </PillButton>
                <PillButton
                  size="sm"
                  pending={busy}
                  variant="danger"
                  onClick={() => execute(() => deleteAgentMemory(selected.id))}
                >
                  {t("agentMemory.delete")}
                </PillButton>
              </>
            )}
          </>
        )}
      </div>
      {isLoading && <SystemMessage>{t("packages.view.loading")}</SystemMessage>}
      {error && (
        <SystemMessage variant="error">
          {error.message}
          <PillButton size="sm" onClick={() => void refetch()}>
            {t("common.retry")}
          </PillButton>
        </SystemMessage>
      )}
      {feedback.state === "error" && (
        <SystemMessage variant="error">{feedback.reason}</SystemMessage>
      )}
      {edit && (
        <div {...stylex.props(styles.editor)}>
          <TextArea
            placeholder={t("agentMemory.add.placeholder")}
            aria-label={t(edit.type === "add" ? "agentMemory.add" : "agentMemory.editAria")}
            rows={2}
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            pending={busy}
          />
          <div {...stylex.props(styles.controls)}>
            <PillButton
              size="sm"
              variant="solid"
              pending={busy}
              disabled={
                !draft.trim() || (edit.type === "update" && draft.trim() === edit.item.content)
              }
              onClick={() =>
                execute(() =>
                  edit.type === "add"
                    ? addAgentMemory({ scope, cwd, content: draft.trim() })
                    : updateAgentMemoryContent(edit.item.id, draft.trim()),
                )
              }
            >
              {t("agentMemory.save")}
            </PillButton>
            <PillButton size="sm" pending={busy} onClick={() => setEdit(undefined)}>
              {t("agentMemory.cancel")}
            </PillButton>
          </div>
        </div>
      )}
    </div>
  );
}

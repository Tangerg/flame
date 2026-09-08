import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useState } from "react";
import { formatDay } from "@/lib/i18n/relativeTime";
import {
  Badge,
  DataView,
  EmptyState,
  gap,
  Icon,
  IconButton,
  PillButton,
  SectionLabel,
  TextArea,
} from "@/ui";

import { useT } from "@/lib/i18n";
import { useCommandAction } from "@/plugins/sdk";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { useRuntimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { type as typeStep } from "@/styles/tokens.stylex";
import { viewStyles as vs } from "./views/viewStyles";
import {
  addAgentMemory,
  deleteAgentMemory,
  reviewAgentMemory,
  setAgentMemoryPinned,
  updateAgentMemoryContent,
  useAgentMemory,
  type AgentMemoryEntry,
  type AgentMemoryQuery,
} from "@/plugins/builtin/workspace/application/agentMemoryConfig";
import { vocab } from "@/ui";

type Scope = AgentMemoryQuery["scope"];

function OriginBadge({ origin }: { origin: AgentMemoryEntry["origin"] }) {
  const t = useT();
  return (
    <Badge>{origin === "auto" ? t("agentMemory.origin.auto") : t("agentMemory.origin.user")}</Badge>
  );
}

function PendingRow({ item }: { item: AgentMemoryEntry }) {
  const t = useT();
  const { busy, run } = useCommandAction({
    wasRetired: wasGenerationRetired,
    fallback: t("agentMemory.error"),
    source: "knowledge",
  });
  return (
    <div {...stylex.props(vs.lineTop, vs.gutter, vs.rowPadTall)}>
      <div {...stylex.props(vocab.fill)}>
        <div {...stylex.props(vs.body, typeStep.uiMd)}>{item.content}</div>
        <div {...stylex.props(vs.metaLine)}>
          <OriginBadge origin={item.origin} />
          {item.sessionId && (
            <span
              {...stylex.props(vocab.truncate, vocab.faint, typeStep.uiSm)}
              title={item.sessionId}
            >
              {t("agentMemory.fromSession")}
            </span>
          )}
        </div>
      </div>
      <div {...stylex.props(vs.actions)}>
        <PillButton
          size="sm"
          variant="danger"
          disabled={busy}
          onClick={() => run(() => reviewAgentMemory(item.id, "reject"))}
        >
          {t("agentMemory.reject")}
        </PillButton>
        <PillButton
          size="sm"
          variant="solid"
          disabled={busy}
          onClick={() => run(() => reviewAgentMemory(item.id, "approve"))}
        >
          {t("agentMemory.approve")}
        </PillButton>
      </div>
    </div>
  );
}

function ActiveRow({ item }: { item: AgentMemoryEntry }) {
  const t = useT();
  const { busy, run } = useCommandAction({
    wasRetired: wasGenerationRetired,
    fallback: t("agentMemory.error"),
    source: "knowledge",
  });
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(item.content);
  const dirty = editing && draft.trim() !== "" && draft !== item.content;

  const save = () => {
    if (!dirty) return;
    run(async () => {
      await updateAgentMemoryContent(item.id, draft.trim());
      setEditing(false);
    });
  };

  return (
    <div {...stylex.props(vocab.column, vs.gutter, vs.rowPadTall)}>
      <div {...stylex.props(vs.lineTop)}>
        <div {...stylex.props(vocab.fill)}>
          {editing ? (
            <TextArea
              aria-label={t("agentMemory.editAria")}
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              spellCheck={false}
              rows={3}
              ink="soft"
            />
          ) : (
            <div {...stylex.props(vs.body, typeStep.uiMd)}>{item.content}</div>
          )}
          <div {...stylex.props(vs.metaLine)}>
            {item.pinned && (
              // The accent is a functional highlight — play, active, CTA — not an ink for
              // prose: on a card in dark it measures 3.4:1, below AA at this size. The mark
              // keeps it, where 3:1 is the bar a graphic answers to; the word does not.
              <span {...stylex.props(vs.pinLine, vocab.muted, typeStep.uiSm)}>
                <Icon name="star" size="xs" className={stylex.props(vocab.accent).className} />
                {t("agentMemory.pinnedLabel")}
              </span>
            )}
            <OriginBadge origin={item.origin} />
            {item.updatedAt && (
              <span {...stylex.props(vocab.truncate, vocab.faint, typeStep.uiSm)}>
                {t("agentMemory.updated")} {formatDay(item.updatedAt)}
              </span>
            )}
          </div>
        </div>
        {!editing && (
          <div {...stylex.props(vs.actionsTight)}>
            <IconButton
              icon="star"
              size="sm"
              active={item.pinned}
              disabled={busy}
              aria-label={item.pinned ? t("agentMemory.unpin") : t("agentMemory.pin")}
              onClick={() => run(() => setAgentMemoryPinned(item.id, !item.pinned))}
            />
            <IconButton
              icon="edit"
              size="sm"
              disabled={busy}
              aria-label={t("agentMemory.edit")}
              onClick={() => {
                setDraft(item.content);
                setEditing(true);
              }}
            />
            <IconButton
              icon="trash"
              size="sm"
              disabled={busy}
              aria-label={t("agentMemory.delete")}
              onClick={() => run(() => deleteAgentMemory(item.id))}
            />
          </div>
        )}
      </div>
      {editing && (
        <div {...stylex.props(vs.formLine)}>
          <PillButton size="sm" variant="accent" disabled={!dirty || busy} onClick={save}>
            {t("agentMemory.save")}
          </PillButton>
          <PillButton size="sm" disabled={busy} onClick={() => setEditing(false)}>
            {t("agentMemory.cancel")}
          </PillButton>
        </div>
      )}
    </div>
  );
}

function AddMemory({ scope, cwd }: { scope: Scope; cwd?: string }) {
  const t = useT();
  const { busy, run } = useCommandAction({
    wasRetired: wasGenerationRetired,
    fallback: t("agentMemory.error"),
    source: "knowledge",
  });
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState("");
  const canSave = draft.trim() !== "";

  if (!open) {
    return (
      <div {...stylex.props(vs.gutter, vs.sectionPad)}>
        <PillButton size="sm" variant="outlined" onClick={() => setOpen(true)}>
          <Icon name="plus" size="xs" />
          {t("agentMemory.add")}
        </PillButton>
      </div>
    );
  }

  const submit = () => {
    if (!canSave) return;
    run(async () => {
      await addAgentMemory({ scope, cwd, content: draft.trim() });
      setDraft("");
      setOpen(false);
    });
  };

  return (
    <div {...stylex.props(vocab.column, gap.s2, vs.gutter, vs.padBottom)}>
      <TextArea
        aria-label={t("agentMemory.add")}
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        placeholder={t("agentMemory.add.placeholder")}
        spellCheck={false}
        rows={2}
        ink="soft"
      />
      <div {...stylex.props(vocab.line, vocab.min)}>
        <PillButton size="sm" variant="accent" disabled={!canSave || busy} onClick={submit}>
          {t("agentMemory.save")}
        </PillButton>
        <PillButton
          size="sm"
          disabled={busy}
          onClick={() => {
            setDraft("");
            setOpen(false);
          }}
        >
          {t("agentMemory.cancel")}
        </PillButton>
      </div>
    </div>
  );
}

function ScopeToggle({ scope, onChange }: { scope: Scope; onChange: (s: Scope) => void }) {
  const t = useT();
  const scopes: Scope[] = ["project", "user"];
  return (
    <div {...stylex.props(vs.filterLine, vs.gutter, vs.statusPad)}>
      {scopes.map((s) => (
        <PillButton
          key={s}
          size="sm"
          variant={scope === s ? "accent" : "outlined"}
          onClick={() => onChange(s)}
        >
          {s === "project" ? t("agentMemory.scope.project") : t("agentMemory.scope.user")}
        </PillButton>
      ))}
    </div>
  );
}

export function AgentMemoryTab() {
  const t = useT();
  const [scope, setScope] = useState<Scope>("project");
  const workspace = useActiveSessionWorkspace();
  const cwd = workspace.status === "ready" ? workspace.cwd : undefined;
  const available = useRuntimeCapability("agentMemory");
  const projectResolving = scope === "project" && workspace.status === "resolving";
  const enabled = available && (scope === "user" || (workspace.status === "ready" && Boolean(cwd)));
  const { data, isLoading, isError, refetch } = useAgentMemory(enabled, scope, cwd);
  const items = data ?? [];
  const pending = items.filter((m) => m.status === "pending");
  const active = items.filter((m) => m.status === "active");

  if (!available) {
    return (
      <WorkspaceViewLayout icon="book" titleStrong title="agentMemory.title" scrollClassName="py-1">
        <EmptyState
          icon="book"
          title={t("agentMemory.unavailable.title")}
          sub={t("agentMemory.unavailable.sub")}
        />
      </WorkspaceViewLayout>
    );
  }

  return (
    <WorkspaceViewLayout
      icon="book"
      titleStrong
      title="agentMemory.title"
      sub={t("agentMemory.sub", { pending: pending.length, active: active.length })}
      scrollClassName="py-1"
    >
      <ScopeToggle scope={scope} onChange={setScope} />
      {enabled && <AddMemory scope={scope} cwd={cwd} />}
      <DataView
        items={items}
        isLoading={(enabled && isLoading) || projectResolving}
        isError={isError}
        onRetry={refetch}
        skeletonCount={3}
        empty={
          enabled
            ? { icon: "book", title: t("agentMemory.empty.title"), sub: t("agentMemory.empty.sub") }
            : {
                icon: "book",
                title: t("agentMemory.noProject.title"),
                sub: t("agentMemory.noProject.sub"),
              }
        }
      >
        {() => (
          <div {...stylex.props(vocab.column, gap.s4)}>
            {pending.length > 0 && (
              <div {...stylex.props(vocab.column)}>
                <div {...stylex.props(vs.gutter, vs.sectionPad)}>
                  <SectionLabel className={stylex.props(vs.sectionLabel).className}>
                    {t("agentMemory.section.pending")}
                  </SectionLabel>
                </div>
                {pending.map((m) => (
                  <PendingRow key={m.id} item={m} />
                ))}
              </div>
            )}
            {active.length > 0 && (
              <div {...stylex.props(vocab.column)}>
                <div {...stylex.props(vs.gutter, vs.sectionPad)}>
                  <SectionLabel className={stylex.props(vs.sectionLabel).className}>
                    {t("agentMemory.section.active")}
                  </SectionLabel>
                </div>
                {active.map((m) => (
                  <ActiveRow key={m.id} item={m} />
                ))}
              </div>
            )}
          </div>
        )}
      </DataView>
    </WorkspaceViewLayout>
  );
}

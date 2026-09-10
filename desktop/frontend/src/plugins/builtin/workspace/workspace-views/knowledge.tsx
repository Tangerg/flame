import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useId, useLayoutEffect, useMemo, useRef, useState } from "react";
import { formatDateTime } from "@/lib/i18n/relativeTime";
import {
  Badge,
  Collapsible,
  DataView,
  Icon,
  PillButton,
  Pressable,
  TextArea,
  chevron,
  gap,
  vocab,
} from "@/ui";
import { useT } from "@/lib/i18n";
import { face, type as typeStep } from "@/styles/tokens.stylex";
import { viewStyles as vs } from "./views/viewStyles";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { notifyError } from "@/plugins/sdk";
import {
  KnowledgeDraft,
  loadWorkspaceKnowledge,
  isWorkspaceKnowledgeRevisionConflict,
  saveWorkspaceKnowledge,
} from "@/plugins/builtin/workspace/application/knowledge";
import { useWorkspaceKnowledge } from "@/plugins/builtin/workspace/application/workspaceQueries";
import {
  type WorkspaceKnowledgeRowViewModel,
  workspaceKnowledgeViewModel,
} from "@/plugins/builtin/workspace/application/workspaceCatalogViewModel";
import { useWorkspaceCapability } from "@/plugins/builtin/workspace/application/workspaceCapabilities";

function KnowledgeRow({ row, cwd }: { row: WorkspaceKnowledgeRowViewModel; cwd?: string }) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const panelId = useId();
  const listedDocument = useMemo(
    () => ({
      content: row.content,
      revision: row.revision,
      ...(row.updatedAt ? { updatedAt: row.updatedAt } : {}),
    }),
    [row.content, row.revision, row.updatedAt],
  );
  const latestListedDocument = useRef(listedDocument);
  useLayoutEffect(() => {
    latestListedDocument.current = listedDocument;
  }, [listedDocument]);
  const [storedEditor, setEditor] = useState(() => KnowledgeDraft.open(listedDocument));
  const editor = storedEditor.reconcile(listedDocument);
  const [saving, setSaving] = useState(false);
  const savingRef = useRef(false);
  const dirty = editor.dirty;

  const toggle = (): void => {
    setOpen((current) => !current);
  };

  const save = (): void => {
    if (!dirty || savingRef.current) return;
    const savedContent = editor.draft;
    const expectedRevision = editor.revision;
    savingRef.current = true;
    setSaving(true);
    saveWorkspaceKnowledge({
      scope: row.scope,
      cwd,
      content: savedContent,
      expectedRevision,
    })
      .then((saved) => {
        setEditor((current) => current.settleSave(saved, latestListedDocument.current));
      })
      .catch(async (error: unknown) => {
        if (wasGenerationRetired(error)) return;
        if (isWorkspaceKnowledgeRevisionConflict(error)) {
          try {
            const latest = await loadWorkspaceKnowledge({ scope: row.scope, cwd });
            setEditor((current) => current.rebase(latest));
          } catch (readError) {
            if (wasGenerationRetired(readError)) return;
          }
        }
        notifyError(t("knowledge.saveError"), {
          description: error instanceof Error ? error.message : String(error),
          source: "knowledge",
        });
      })
      .finally(() => {
        savingRef.current = false;
        setSaving(false);
      });
  };

  return (
    <div {...stylex.props(vocab.column)}>
      <Pressable
        type="button"
        aria-expanded={open}
        aria-controls={panelId}
        onClick={toggle}
        className={stylex.props(vs.discloseRow, vs.gutter, vs.rowPad, vs.wash).className}
      >
        <Icon
          name="chevron-down"
          size="xs"
          className={stylex.props(chevron.base, vocab.faint, !open && chevron.shut).className}
        />
        <span {...stylex.props(vocab.ink, vocab.truncate, typeStep.uiMd, face.mono)}>
          {row.path}
        </span>
        <Badge>{t(row.scopeLabelKey)}</Badge>
      </Pressable>
      <Collapsible open={open}>
        <div id={panelId} {...stylex.props(vocab.column, gap.s2, vs.gutter, vs.editorInset)}>
          <TextArea
            aria-label={t("knowledge.aria", { path: row.path })}
            value={editor.draft}
            onChange={(e) =>
              setEditor((current) => current.reconcile(listedDocument).edit(e.target.value))
            }
            spellCheck={false}
            rows={12}
            ink="soft"
          />
          <div {...stylex.props(vocab.line, vocab.min)}>
            <PillButton
              size="sm"
              variant="accent"
              disabled={!dirty}
              pending={saving}
              onClick={save}
            >
              {saving ? t("knowledge.saving") : t("knowledge.save")}
            </PillButton>
            <PillButton
              size="sm"
              disabled={!dirty}
              pending={saving}
              onClick={() => setEditor((current) => current.reconcile(listedDocument).revert())}
            >
              {t("knowledge.revert")}
            </PillButton>
            {editor.updatedAt && (
              <span {...stylex.props(vs.pushEnd, vocab.faint, typeStep.uiXs)}>
                {t("knowledge.updated")} {formatDateTime(editor.updatedAt)}
              </span>
            )}
          </div>
        </div>
      </Collapsible>
    </div>
  );
}

export function KnowledgeTab() {
  const t = useT();
  const knowledgeEnabled = useWorkspaceCapability("knowledge");
  const workspace = useActiveSessionWorkspace();
  const cwd = workspace.status === "ready" ? workspace.cwd : undefined;
  const { data, isLoading, isError, refetch } = useWorkspaceKnowledge(
    knowledgeEnabled && workspace.status === "ready" ? { cwd } : undefined,
  );
  const view = workspaceKnowledgeViewModel(data ?? [], knowledgeEnabled);

  return (
    <WorkspaceViewLayout
      icon="filetext"
      title="knowledge.title"
      sub={view.enabled ? t("knowledge.scopes", { count: view.count }) : t("knowledge.off")}
    >
      <DataView
        items={view.rows}
        isLoading={view.enabled && (isLoading || workspace.status === "resolving")}
        isError={isError}
        onRetry={refetch}
        skeletonCount={2}
        empty={
          knowledgeEnabled
            ? {
                icon: "filetext",
                title: t("knowledge.empty.title"),
                sub: t("knowledge.empty.sub"),
              }
            : {
                icon: "filetext",
                title: t("knowledge.disabled.title"),
                sub: t("knowledge.disabled.sub"),
              }
        }
      >
        {(rows) => (
          <div {...stylex.props(vocab.column)}>
            {rows.map((m) => (
              <KnowledgeRow key={`${cwd ?? ""}:${m.id}`} row={m} cwd={cwd} />
            ))}
          </div>
        )}
      </DataView>
    </WorkspaceViewLayout>
  );
}

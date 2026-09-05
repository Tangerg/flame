import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useCallback, useRef, useState } from "react";
import { Badge, Collapsible, DataView, PillButton, Tag, TextButton, Well } from "@/ui";
import { useT } from "@/lib/i18n";
import { notifyError } from "@/plugins/sdk";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import {
  useSkillProposals,
  type SkillProposal,
} from "@/plugins/builtin/workspace/application/workspaceQueries";
import {
  approveSkillProposal,
  rejectSkillProposal,
} from "@/plugins/builtin/workspace/application/skillCuration";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";

export function SkillProposalsTab() {
  const t = useT();
  const workspace = useActiveSessionWorkspace();
  const { data, isLoading, isError, refetch } = useSkillProposals(
    workspace.status === "ready" ? { cwd: workspace.cwd } : undefined,
  );
  const proposals = data ?? [];

  return (
    <WorkspaceViewLayout
      icon="sparkle"
      titleStrong
      title="skillProposals.title"
      sub={t("skillProposals.sub", { count: proposals.length })}
      scrollClassName="py-1"
    >
      <DataView
        items={proposals}
        isLoading={isLoading || workspace.status === "resolving"}
        isError={isError}
        onRetry={refetch}
        skeletonCount={3}
        empty={{
          icon: "sparkle",
          title: t("skillProposals.empty.title"),
          sub: t("skillProposals.empty.sub"),
        }}
      >
        {(rows) => (
          <div className="flex flex-col py-1">
            {rows.map((proposal) => (
              <SkillProposalRow key={`${proposal.name} ${proposal.revision}`} proposal={proposal} />
            ))}
          </div>
        )}
      </DataView>
    </WorkspaceViewLayout>
  );
}

function SkillProposalRow({ proposal }: { proposal: SkillProposal }) {
  const t = useT();
  const actionPending = useRef(false);
  const [busy, setBusy] = useState(false);
  const [reading, setReading] = useState(false);

  const act = useCallback(
    async (run: () => Promise<void>) => {
      if (actionPending.current) return;
      actionPending.current = true;
      setBusy(true);
      try {
        await run();
      } catch (error) {
        if (!wasGenerationRetired(error)) {
          notifyError(error instanceof Error ? error.message : t("skillProposals.error"), {
            source: "skills",
          });
        }
      } finally {
        actionPending.current = false;
        setBusy(false);
      }
    },
    [t],
  );

  const handle = {
    workspace: proposal.workspace,
    name: proposal.name,
    revision: proposal.revision,
    scope: proposal.scope,
  };

  return (
    <div className="px-4 py-2.5">
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <div className="truncate text-ui-md font-semibold text-fg">{proposal.name}</div>
            <Tag className="tabular-nums">{proposal.revision.slice(0, 8)}</Tag>
            <Badge>{t(`skillProposals.scope.${proposal.scope}`)}</Badge>
            {proposal.revises && <Badge tone="warning">{t("skillProposals.revises")}</Badge>}
          </div>
          {proposal.description && (
            <div className="mt-0.5 text-ui-sm leading-body text-fg-muted">
              {proposal.description}
            </div>
          )}
          <div
            className="mt-1 truncate text-ui-sm text-fg-faint"
            title={proposal.sourceSession || undefined}
          >
            {t(`skillProposals.origin.${proposal.origin}`)}
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <PillButton
            size="sm"
            variant="danger"
            disabled={busy}
            onClick={() => void act(() => rejectSkillProposal(handle))}
          >
            {t("skillProposals.reject")}
          </PillButton>
          <PillButton
            size="sm"
            variant="solid"
            disabled={busy}
            onClick={() => void act(() => approveSkillProposal(handle))}
          >
            {t("skillProposals.approve")}
          </PillButton>
        </div>
      </div>
      {proposal.instructions && (
        <>
          <TextButton
            size="sm"
            className="mt-1.5"
            aria-expanded={reading}
            onClick={() => setReading((open) => !open)}
          >
            {reading ? t("skillProposals.hideBody") : t("skillProposals.readBody")}
          </TextButton>
          <Collapsible open={reading}>
            <Well className="mt-1.5">{proposal.instructions}</Well>
          </Collapsible>
        </>
      )}
    </div>
  );
}

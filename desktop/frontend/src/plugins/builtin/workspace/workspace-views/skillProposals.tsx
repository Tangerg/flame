import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useCallback, useRef, useState } from "react";
import { Badge, Collapsible, DataView, PillButton, Tag, TextButton, vocab, Well } from "@/ui";
import { useT } from "@/lib/i18n";
import { type as typeStep } from "@/styles/tokens.stylex";
import { viewStyles as vs } from "./views/viewStyles";
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
      title="skillProposals.title"
      sub={t("skillProposals.sub", { count: proposals.length })}
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
          <div {...stylex.props(vocab.column, vs.padBlockSm)}>
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
    <div {...stylex.props(vs.gutter, vs.rowPadTall)}>
      <div {...stylex.props(vs.lineTop)}>
        <div {...stylex.props(vocab.fill)}>
          <div {...stylex.props(vs.titleLine)}>
            <div {...stylex.props(vs.title, vocab.truncate, typeStep.uiMd)}>{proposal.name}</div>
            <Tag className={stylex.props(vocab.figures).className}>
              {proposal.revision.slice(0, 8)}
            </Tag>
            <Badge>{t(`skillProposals.scope.${proposal.scope}`)}</Badge>
            {proposal.revises && <Badge tone="warning">{t("skillProposals.revises")}</Badge>}
          </div>
          {proposal.description && (
            <div {...stylex.props(vs.description, typeStep.uiSm)}>{proposal.description}</div>
          )}
          <div
            {...stylex.props(vs.origin, vocab.truncate, typeStep.uiSm)}
            title={proposal.sourceSession || undefined}
          >
            {t(`skillProposals.origin.${proposal.origin}`)}
          </div>
        </div>
        <div {...stylex.props(vs.actions)}>
          <PillButton
            size="sm"
            variant="danger"
            pending={busy}
            onClick={() => void act(() => rejectSkillProposal(handle))}
          >
            {t("skillProposals.reject")}
          </PillButton>
          <PillButton
            size="sm"
            variant="solid"
            pending={busy}
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
            className={stylex.props(vocab.afterLine).className}
            aria-expanded={reading}
            onClick={() => setReading((open) => !open)}
          >
            {reading ? t("skillProposals.hideBody") : t("skillProposals.readBody")}
          </TextButton>
          <Collapsible open={reading}>
            <Well className={stylex.props(vocab.afterLine).className}>{proposal.instructions}</Well>
          </Collapsible>
        </>
      )}
    </div>
  );
}

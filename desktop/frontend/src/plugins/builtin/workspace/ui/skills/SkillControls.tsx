import { useState } from "react";
import * as stylex from "@stylexjs/stylex";
import { isCancellationReason } from "dougong";
import { DropdownMenu, SelectTrigger, PillButton, ConfirmDialog, SystemMessage, vocab } from "@/ui";
import { space } from "@/styles/tokens.stylex";
import { useT } from "@/lib/i18n";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useCommandAction, notifyError } from "@/plugins/sdk";
import {
  useWorkspaceSkills,
  useManagedSkills,
  useSkillProposals,
  type SkillProposal,
} from "../../application/workspaceQueries";
import {
  archiveSkill,
  restoreSkill,
  approveSkillProposal,
  rejectSkillProposal,
} from "../../application/skillCuration";
import { SkillProposalRevisionConflictError } from "../../application/ports/skillCurationGateway";

const styles = stylex.create({
  root: { padding: space.s3, flexShrink: 0 },
  controls: { display: "flex", flexWrap: "wrap", gap: space.s2 },
  identity: { overflowWrap: "anywhere" },
  instructions: {
    display: "block",
    whiteSpace: "pre-wrap",
    overflowWrap: "anywhere",
    maxHeight: "40vh",
    overflow: "auto",
    marginTop: space.s2,
  },
});
type Lifetime = { signal: AbortSignal; onSaved(): void };
type Failure = { isLoading: boolean; error: Error | null; refetch(): unknown };
function ReadFailure({ isLoading, error, refetch }: Failure) {
  const t = useT();
  return (
    <>
      {isLoading && <SystemMessage>{t("packages.view.loading")}</SystemMessage>}
      {error && (
        <SystemMessage variant="error">
          {error.message}
          <PillButton size="sm" onClick={() => void refetch()}>
            {t("common.retry")}
          </PillButton>
        </SystemMessage>
      )}
    </>
  );
}

export function SkillInspectionControl({
  cwd,
  name,
  onInspect,
}: {
  cwd: string;
  name?: string;
  onInspect(name?: string): void;
}) {
  const t = useT();
  const query = useWorkspaceSkills({ cwd });
  const skills = query.data?.skills ?? [];
  return (
    <div {...stylex.props(styles.root)}>
      <DropdownMenu.Root>
        <DropdownMenu.Trigger
          render={
            <SelectTrigger
              label={name ?? t("skills.inspect")}
              aria-label={t("skills.inspect")}
              disabled={query.isLoading || Boolean(query.error)}
            />
          }
        />
        <DropdownMenu.Content align="start">
          <DropdownMenu.Item layout="pickPlain" onClick={() => onInspect(undefined)}>
            {t("skills.tab.available")}
          </DropdownMenu.Item>
          {skills.map((skill) => (
            <DropdownMenu.Item
              key={skill.name}
              layout="pickPlain"
              onClick={() => onInspect(skill.name)}
            >
              {skill.name} · {skill.scope}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Root>
      <ReadFailure {...query} />
    </div>
  );
}

export function SkillLibraryControls({ signal, onSaved }: Lifetime) {
  const t = useT();
  const query = useManagedSkills();
  const [name, setName] = useState<string>();
  const selected = query.data?.find((skill) => skill.name === name);
  const { busy, run } = useCommandAction({
    wasRetired: (error) => wasGenerationRetired(error) || isCancellationReason(signal, error),
    fallback: t("skillLibrary.error"),
    source: "skills",
  });
  return (
    <div {...stylex.props(styles.root)}>
      <div {...stylex.props(styles.controls)}>
        <DropdownMenu.Root>
          <DropdownMenu.Trigger
            render={
              <SelectTrigger
                label={name ?? t("skillLibrary.select")}
                aria-label={t("skillLibrary.select")}
                pending={busy}
                disabled={query.isLoading || Boolean(query.error)}
              />
            }
          />
          <DropdownMenu.Content align="start">
            {query.data?.map((skill) => (
              <DropdownMenu.Item
                key={skill.name}
                layout="pickPlain"
                onClick={() => setName(skill.name)}
              >
                {skill.name} · {t(`skillLibrary.section.${skill.lifecycle}`)}
              </DropdownMenu.Item>
            ))}
          </DropdownMenu.Content>
        </DropdownMenu.Root>
        {selected && !query.error && !query.isLoading && (
          <PillButton
            size="sm"
            pending={busy}
            variant={selected.lifecycle === "archived" ? "outlined" : "danger"}
            onClick={() =>
              run(async () => {
                signal.throwIfAborted();
                await (selected.lifecycle === "archived"
                  ? restoreSkill(selected.name)
                  : archiveSkill(selected.name));
                if (!signal.aborted) onSaved();
              })
            }
          >
            {t(selected.lifecycle === "archived" ? "skillLibrary.restore" : "skillLibrary.archive")}
          </PillButton>
        )}
      </div>
      <p>{t("skillLibrary.archiveEffect")}</p>
      <ReadFailure {...query} />
    </div>
  );
}

export function SkillProposalControls({ cwd, signal, onSaved }: Lifetime & { cwd: string }) {
  const t = useT();
  const query = useSkillProposals({ cwd });
  const [selection, setSelection] = useState<string>();
  const [review, setReview] = useState<{
    proposal: SkillProposal;
    decision: "approve" | "reject";
  }>();
  const selected = query.data?.find(
    (proposal) => `${proposal.scope}:${proposal.name}` === selection,
  );
  const { busy, run } = useCommandAction({
    wasRetired: (error) => wasGenerationRetired(error) || isCancellationReason(signal, error),
    fallback: t("skillProposals.error"),
    source: "skills",
  });
  const decide = (captured: NonNullable<typeof review>) =>
    run(async () => {
      signal.throwIfAborted();
      const { workspace, scope, name, revision } = captured.proposal;
      try {
        await (captured.decision === "approve" ? approveSkillProposal : rejectSkillProposal)({
          workspace,
          scope,
          name,
          revision,
        });
        if (!signal.aborted) onSaved();
      } catch (error) {
        if (!(error instanceof SkillProposalRevisionConflictError)) throw error;
        if (!signal.aborted) {
          setSelection(undefined);
          onSaved();
          notifyError(t("skillProposals.conflict"), { source: "skills" });
        }
      }
    });
  return (
    <div {...stylex.props(styles.root)}>
      <div {...stylex.props(styles.controls)}>
        <DropdownMenu.Root>
          <DropdownMenu.Trigger
            render={
              <SelectTrigger
                label={selected?.name ?? t("skillProposals.select")}
                aria-label={t("skillProposals.select")}
                pending={busy}
                disabled={query.isLoading || Boolean(query.error)}
              />
            }
          />
          <DropdownMenu.Content align="start">
            {query.data?.map((proposal) => (
              <DropdownMenu.Item
                key={`${proposal.scope}:${proposal.name}`}
                layout="pickPlain"
                onClick={() => setSelection(`${proposal.scope}:${proposal.name}`)}
              >
                {proposal.name} · {t(`skillProposals.scope.${proposal.scope}`)} ·{" "}
                {proposal.revision.slice(0, 8)}
              </DropdownMenu.Item>
            ))}
          </DropdownMenu.Content>
        </DropdownMenu.Root>
        {selected &&
          !query.error &&
          !query.isLoading &&
          (["reject", "approve"] as const).map((decision) => (
            <PillButton
              key={decision}
              size="sm"
              pending={busy}
              variant={decision === "reject" ? "danger" : "solid"}
              onClick={() => setReview({ proposal: selected, decision })}
            >
              {t(`skillProposals.${decision}`)}
            </PillButton>
          ))}
      </div>
      <ReadFailure {...query} />
      <ConfirmDialog
        open={Boolean(review)}
        onOpenChange={(open) => {
          if (!open) setReview(undefined);
        }}
        title={review && `${t(`skillProposals.${review.decision}`)} · ${review.proposal.name}`}
        body={
          review && (
            <span {...stylex.props(vocab.column)}>
              <span {...stylex.props(styles.identity)}>
                {review.proposal.workspace} · {review.proposal.scope} · {review.proposal.revision}
              </span>
              <span>{review.proposal.description}</span>
              <span {...stylex.props(styles.instructions)}>{review.proposal.instructions}</span>
            </span>
          )
        }
        confirmLabel={review ? t(`skillProposals.${review.decision}`) : t("skillProposals.approve")}
        cancelLabel={t("common.cancel")}
        destructive={review?.decision === "reject"}
        onConfirm={() => {
          if (review) decide(review);
        }}
      />
    </div>
  );
}

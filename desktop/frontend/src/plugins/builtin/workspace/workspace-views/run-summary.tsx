import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { Badge, DiffStat, EmptyState, FilePath, Icon, IconButton, toneInk, vocab } from "@/ui";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useCopyFeedback } from "@/lib/useCopyFeedback";
import { buildPlaintext } from "@/plugins/builtin/agent/public/runDigest";
import { useT } from "@/lib/i18n";
import { face, type as typeStep } from "@/styles/tokens.stylex";
import { viewStyles as vs } from "./views/viewStyles";
import { useLatestRunDigest } from "@/plugins/builtin/workspace/presentation/runSummaryView";
import {
  runSummaryApprovalBadge,
  runSummaryCommandTone,
  runSummaryViewModel,
} from "@/plugins/builtin/workspace/application/runSummaryViewModel";

function Section({
  title,
  count,
  children,
}: {
  title: string;
  count: number;
  children: ReactNode;
}) {
  const t = useT();
  if (count === 0) return null;
  return (
    <div {...stylex.props(vs.gutter, vs.sectionOuterPad)}>
      <div {...stylex.props(vs.sectionHead)}>
        <span {...stylex.props(vs.title, typeStep.uiMd)}>{t(title)}</span>
        <span {...stylex.props(vocab.faint, typeStep.uiSm, face.mono)}>{count}</span>
      </div>
      <div {...stylex.props(vs.sectionBody)}>{children}</div>
    </div>
  );
}

export function RunSummaryTab() {
  const t = useT();
  const digest = useLatestRunDigest();
  const copyMaterial = digest ? buildPlaintext(t, digest) : "";
  const { copied, copy } = useCopyFeedback(copyMaterial);

  if (!digest) {
    return (
      <WorkspaceViewLayout
        scrollInset="flush"
        icon="check"
        title="runSummary.title"
        sub={t("runSummary.noRuns")}
      >
        <EmptyState
          icon="check"
          title={t("runSummary.empty.title")}
          sub={t("runSummary.empty.sub")}
        />
      </WorkspaceViewLayout>
    );
  }

  const view = runSummaryViewModel(t, digest);

  return (
    <WorkspaceViewLayout
      scrollInset="flush"
      icon="check"
      title="runSummary.title"
      sub={view.subtext}
      actions={
        <IconButton
          icon={copied ? "check" : "copy"}
          iconSize="sm"
          title={t(copied ? "runSummary.copied" : "runSummary.copy")}
          onClick={() => void copy()}
        />
      }
    >
      <div {...stylex.props(vs.gutter, vs.statusPad)}>
        <Badge
          tone={view.statusBadge.tone}
          face="mono"
          className={stylex.props(vocab.strong).className}
        >
          {t(view.statusBadge.labelKey)}
        </Badge>
      </div>

      <Section title="runSummary.section.changedFiles" count={view.changedFiles.count}>
        {view.changedFiles.items.map((f) => (
          <div key={f.path} {...stylex.props(vs.entry, vocab.muted, typeStep.uiMd)}>
            <Icon name="filetext" size="xs" className={stylex.props(vocab.faint).className} />
            <FilePath path={f.path} className={stylex.props(vocab.grow, vocab.ink).className} />
            {(f.added !== undefined || f.removed !== undefined) && (
              <DiffStat
                added={f.added ?? 0}
                removed={f.removed ?? 0}
                className={stylex.props(vs.pushEnd, typeStep.uiSm).className}
              />
            )}
          </div>
        ))}
      </Section>

      <Section title="runSummary.section.readFiles" count={view.readFiles.count}>
        {view.readFiles.items.map((p) => (
          <div key={p} {...stylex.props(vs.entry, vocab.muted, typeStep.uiMd)}>
            <Icon name="filetext" size="xs" className={stylex.props(vocab.faint).className} />
            <span {...stylex.props(vocab.truncate)}>{p}</span>
          </div>
        ))}
      </Section>

      <Section title="runSummary.section.commands" count={view.commands.count}>
        {view.commands.items.map((c, i) => (
          <div key={`${c.cmd}:${i}`} {...stylex.props(vs.entry, typeStep.uiMd)}>
            <Icon name="terminal" size="xs" className={stylex.props(vocab.faint).className} />
            <span {...stylex.props(vocab.truncate, toneInk[runSummaryCommandTone(c.status)])}>
              {c.cmd}
            </span>
          </div>
        ))}
      </Section>

      <Section title="runSummary.section.approvals" count={view.approvals.count}>
        {view.approvals.items.map((a, i) => {
          const approval = runSummaryApprovalBadge(a.decision);
          return (
            <div key={`${a.command}:${i}`} {...stylex.props(vs.entry, vocab.muted, typeStep.uiMd)}>
              <Icon name="shield" size="xs" className={stylex.props(vocab.faint).className} />
              <span {...stylex.props(vocab.truncate)}>
                {a.command || t("runSummary.approval.noCommand")}
              </span>
              <span
                {...stylex.props(vs.pushEnd, vocab.strong, toneInk[approval.tone], typeStep.uiXs)}
              >
                {t(approval.labelKey)}
              </span>
            </div>
          );
        })}
      </Section>

      <Section title="runSummary.section.errors" count={view.errors.count}>
        {view.errors.items.map((e, i) => (
          <div key={`${e}:${i}`} {...stylex.props(vs.entryPlain, vocab.negative, typeStep.uiMd)}>
            <Icon name="bug" size="xs" />
            <span {...stylex.props(vocab.wrapText)}>{e}</span>
          </div>
        ))}
      </Section>
    </WorkspaceViewLayout>
  );
}

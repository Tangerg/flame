import { useAgent } from "@/features/agent/public/agent";
import { iconGlyph } from "@/ui/icons/index";
import {
  type ApprovalRuleSummary,
  describeApprovalRuleTool,
  describeApprovalSubject,
} from "@/features/agent/public/approvalPolicy";
import * as stylex from "@stylexjs/stylex";
import { Fragment, useId, useLayoutEffect, useRef, type ReactNode, type RefObject } from "react";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { DataView } from "@/ui/index";
import { TextButton } from "@/ui/index";
import { Badge, IconButton, PanelSection, vocab, handoffFocus } from "@flame/ui";
import {
  forgetApprovalRule,
  forgetApprovalRules,
  useApprovalRuleConfigs,
} from "@/features/approvals/application/approvalConfig";
import { useActiveSessionId } from "@/features/agent/public/session";
import { useCommandAction } from "@/features/command/public/commandAction";
import { Trans, useT, type Translate } from "@/lib/i18n/index";
import {
  color,
  face,
  layout,
  motion,
  radius,
  space,
  surface,
  type as typeStep,
  weight,
} from "@flame/ui/tokens.stylex";

const r = stylex.create({
  // Rows bleed by their inset so rule text starts on the mode choices' edge.
  list: {
    display: "flex",
    flexDirection: "column",
    gap: space.s0_5,
    marginInline: `calc(-1 * ${space.s2})`,
  },
  rule: {
    display: "grid",
    gridTemplateColumns: "minmax(0, 1fr) auto",
    alignItems: "start",
    gap: layout.controlGap,
    borderRadius: radius.row,
    backgroundColor: { default: null, ":hover": surface.hover },
    paddingInline: space.s2,
    paddingBlock: space.s2,
    transitionProperty: "background-color",
    transitionDuration: motion.color,
    transitionTimingFunction: motion.easeState,
  },
  copy: {
    display: "grid",
    gridTemplateColumns: "minmax(0, 1fr)",
    gap: layout.labelGap,
    minWidth: 0,
    overflowWrap: "anywhere",
  },
  identity: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: layout.controlGap,
    minWidth: 0,
  },
  details: { minWidth: 0, whiteSpace: "pre-wrap" },
  verdict: { flexShrink: 0, fontWeight: weight.medium },
  tool: { color: color.fg, minWidth: 0, maxWidth: "100%" },
});

export function RulesSection() {
  const agentAccess = useAgent();

  const t = useT();
  const sessionId = useActiveSessionId();
  const { data, isPending, error, refetch } = useApprovalRuleConfigs(sessionId);
  const { busy, run } = useCommandAction({
    wasRetired: wasGenerationRetired,
    fallback: t("approvals.error.forget"),
  });
  const descriptionPrefix = useId();
  const titleId = `${descriptionPrefix}-title`;
  const context = useRef<HTMLDivElement>(null);
  const source = useRef({ agentAccess, sessionId });
  useLayoutEffect(() => {
    source.current = { agentAccess, sessionId };
  });

  return (
    <PanelSection
      title={<span id={titleId}>{t("approvals.rules")}</span>}
      actions={
        data?.length ? (
          <ClearRules
            context={context}
            busy={busy}
            current={() =>
              source.current.agentAccess === agentAccess && source.current.sessionId === sessionId
            }
            onClick={() => run(() => forgetApprovalRules(agentAccess, data))}
            label={t("approvals.clearAll")}
          />
        ) : undefined
      }
    >
      <div ref={context} role="group" aria-labelledby={titleId} tabIndex={-1} data-focus-inset="">
        <DataView
          items={data}
          isLoading={isPending}
          failure={error}
          onRetry={refetch}
          unsupported={{ icon: "shield" }}
          empty={{
            icon: "shield",
            title: t("approvals.rules.empty"),
            align: "start",
          }}
          error={{ align: "start" }}
        >
          {(rows) => (
            <div {...stylex.props(r.list)}>
              {rows.map((rule, index) => {
                const descriptionId = `${descriptionPrefix}-${index}`;
                return (
                  <div key={rule.id} data-focus-unit="" {...stylex.props(r.rule)}>
                    <div id={descriptionId} {...stylex.props(r.copy)}>
                      <div {...stylex.props(r.identity, typeStep.uiMd)}>
                        <span
                          {...stylex.props(r.verdict, rule.decision === "deny" && vocab.negative)}
                        >
                          {rule.decision === "deny" ? t("approvals.deny") : t("approvals.allow")}
                        </span>
                        <span {...stylex.props(r.tool, face.mono)}>{rule.modelName}</span>
                        {rule.stale ? <Badge tone="warning">{t("approvals.stale")}</Badge> : null}
                      </div>
                      <div {...stylex.props(r.details, vocab.muted, typeStep.uiSm)}>
                        <RuleDetails rule={rule} t={t} />
                      </div>
                    </div>
                    <RuleForget
                      context={context}
                      current={() =>
                        source.current.agentAccess === agentAccess &&
                        source.current.sessionId === sessionId
                      }
                      name={t("approvals.forget", { tool: rule.modelName })}
                      descriptionId={descriptionId}
                      busy={busy}
                      onClick={() => run(() => forgetApprovalRule(agentAccess, rule.id))}
                    />
                  </div>
                );
              })}
            </div>
          )}
        </DataView>
      </div>
    </PanelSection>
  );
}

function RuleForget({
  context,
  current,
  name,
  descriptionId,
  busy,
  onClick,
}: {
  context: RefObject<HTMLDivElement | null>;
  current(): boolean;
  name: string;
  descriptionId: string;
  busy: boolean;
  onClick(): void;
}) {
  const action = useRef<HTMLButtonElement>(null);
  return (
    <IconButton
      ref={action}
      data-rule-forget=""
      icon={iconGlyph("x")}
      size="xs"
      quiet
      styles={vocab.hold}
      aria-label={name}
      aria-describedby={descriptionId}
      aria-busy={busy}
      pending={busy}
      onClick={(event) => {
        if (current()) handoffFocus(event.currentTarget, context.current);
        onClick();
      }}
    />
  );
}

function ClearRules({
  context,
  current,
  busy,
  onClick,
  label,
}: {
  context: RefObject<HTMLDivElement | null>;
  current(): boolean;
  busy: boolean;
  onClick(): void;
  label: string;
}) {
  const action = useRef<HTMLButtonElement>(null);
  return (
    <TextButton
      ref={action}
      pending={busy}
      onClick={(event) => {
        if (current()) handoffFocus(event.currentTarget, context.current);
        onClick();
      }}
    >
      {label}
    </TextButton>
  );
}

function Literal({ text }: { text: string }) {
  return <span {...stylex.props(face.mono)}>{text}</span>;
}

function RuleDetails({ rule, t }: { rule: ApprovalRuleSummary; t: Translate }) {
  const source = describeApprovalRuleTool(rule.tool);
  const subject = describeApprovalSubject(rule.subject, t);
  const parts: ReactNode[] = [t(`approvals.scope.${rule.scope}`)];
  if (rule.dir) parts.push(<Literal text={rule.dir} />);
  if (source) parts.push(<Literal text={source} />);
  if (subject)
    parts.push(
      <Trans
        i18nKey="approvals.subject.entry"
        values={{ kind: subject.label }}
        components={{ value: <Literal text={subject.value} /> }}
      />,
    );
  return parts.map((part, index) => (
    <Fragment key={index}>
      {index > 0 && " · "}
      {part}
    </Fragment>
  ));
}

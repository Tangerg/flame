import * as stylex from "@stylexjs/stylex";
import { toolFamilyId } from "@/lib/toolFamilies";
import { toolIconFor } from "@/features/agent/public/toolIcon";
import { type ApprovalDecision } from "@/features/agent/public/hitl";
import { type RememberScope } from "@/features/agent/public/hitl";
import { useRuntimeCommandsAvailable } from "@/features/connection/public/serviceStatus";
import { useT } from "@/lib/i18n/index";
import { Divider } from "@/ui/index";
import { DropdownMenu } from "@/ui/index";
import { Icon, type IconName } from "@/ui/index";
import { Surface } from "@/ui/index";
import { Well } from "@/ui/index";
import { Button, vocab } from "@flame/ui";
import { useApprovalArgsEditor } from "@/features/chat/message/application/approvalArgsEditor";
import { useApprovalCardActions } from "@/features/chat/message/application/approvalCardActions";
import { ApprovalArgsEditor } from "@/features/chat/message/ui/cards/ApprovalArgsEditor";
import { approvalHeadline } from "@/features/chat/message/ui/cards/approvalHeadline";
import { HitlSettledRow } from "@/features/chat/message/ui/cards/HitlCard";
import { type as typeStep } from "@flame/ui/tokens.stylex";
import { messageStyles as ms } from "@/features/chat/message/ui/messageStyles";
const ac = stylex.create({
  actionLabel: { display: "flex", alignItems: "center" },
});

interface Props {
  /** The Run to resume; present only while this approval awaits an answer. */
  resumeRunId?: string;
  decision?: ApprovalDecision;
  toolName?: string;
  cmd: string;
  reason: string;
  itemId?: string;
  args?: Record<string, unknown>;
  rememberable?: boolean;
}

const REMEMBER_ACTIONS: readonly {
  scope: RememberScope;
  labelKey: string;
}[] = [
  { scope: "session", labelKey: "approval.action.allowSession" },
  { scope: "project", labelKey: "approval.action.allowProject" },
  { scope: "global", labelKey: "approval.action.allowGlobal" },
];

export function ApprovalCard({
  resumeRunId,
  decision,
  toolName,
  cmd,
  reason,
  itemId,
  args,
  rememberable = false,
}: Props) {
  const t = useT();
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const hasArgs = args !== undefined;
  const argsEditor = useApprovalArgsEditor({
    originalArgs: hasArgs ? JSON.stringify(args, null, 2) : "",
  });
  const { disabled, approve, decline } = useApprovalCardActions({
    resumeRunId,
    itemId,
    argsEditor: hasArgs ? argsEditor : undefined,
    runtimeAvailable,
  });

  if (decision === "approve") {
    return <HitlSettledRow label={t("approval.settled.approved")} />;
  }
  if (decision === "deny") {
    return <Divider icon={<Icon name="x" size="xs" />}>{t("approval.settled.declined")}</Divider>;
  }
  if (resumeRunId === undefined) return null;

  const identity = approvalIdentity(t, toolName);
  const title = reason.trim() || approvalHeadline(t, toolName);

  return (
    <Surface
      variant="card"
      inset="none"
      data-slot="approval-surface"
      className={stylex.props(ms.cardClip).className}
    >
      <div {...stylex.props(ms.cardHead)}>
        <div {...stylex.props(ms.identity, typeStep.uiSm)}>
          <Icon
            name={identity.icon}
            size="sm"
            className={stylex.props(vocab.hold, vocab.faint).className}
          />
          <span {...stylex.props(vocab.truncate)}>{identity.label}</span>
        </div>
        <div {...stylex.props(ms.cardPrompt, typeStep.uiMd)}>{title}</div>
      </div>

      {(cmd.trim() || hasArgs) && (
        <div {...stylex.props(ms.cardBody)}>
          {cmd.trim() && (
            <Well as="code" ink="strong" cap="lg">
              {cmd}
            </Well>
          )}
          {hasArgs && (
            <ApprovalArgsEditor
              editing={argsEditor.editing}
              argsText={argsEditor.argsText}
              invalid={argsEditor.invalid}
              onEditToggle={argsEditor.setEditing}
              onTextChange={argsEditor.setArgsText}
            />
          )}
        </div>
      )}

      <div {...stylex.props(ms.cardActions)}>
        <Button variant="outline" size="sm" disabled={disabled} onClick={decline}>
          {t("approval.action.deny")}
        </Button>
        <div
          role="group"
          aria-label={t("approval.action.allowOptions")}
          className={stylex.props(ac.actionLabel).className}
        >
          <Button
            variant="primary"
            size="sm"
            disabled={disabled}
            join={rememberable ? "start" : undefined}
            onClick={() => approve()}
          >
            {t("approval.action.allowOnce")}
          </Button>
          {rememberable && (
            <DropdownMenu.Root>
              <DropdownMenu.Trigger
                render={
                  <Button
                    variant="primary"
                    size="icon-sm"
                    disabled={disabled}
                    aria-label={t("approval.action.allowOptions")}
                    join="end"
                  >
                    <Icon name="chevron-down" size="xs" />
                  </Button>
                }
              />
              <DropdownMenu.Content align="end" sideOffset={4}>
                {REMEMBER_ACTIONS.map((action) => (
                  <DropdownMenu.Item
                    key={action.scope}
                    onClick={() => approve(action.scope)}
                    layout="pickPlain"
                  >
                    <span {...stylex.props(vocab.truncate)}>{t(action.labelKey)}</span>
                  </DropdownMenu.Item>
                ))}
              </DropdownMenu.Content>
            </DropdownMenu.Root>
          )}
        </div>
      </div>
    </Surface>
  );
}

function approvalIdentity(
  t: (key: string, params?: Record<string, string | number>) => string,
  toolName: string | undefined,
): { icon: IconName; label: string } {
  if (!toolName) {
    return { icon: "shield", label: t("approval.identity.permission") };
  }
  const family = toolFamilyId(toolName);
  return {
    icon: toolIconFor(toolName),
    label: family === undefined ? toolName : t(`tools.family.${family}`),
  };
}

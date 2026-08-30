import { Divider, Icon } from "@/ui";

// QuestionCard keeps its own compact HITL shell. Approval requests deliberately
// do not share it: Codex gives approvals a larger, role-specific request surface
// whose identity, material and scoped actions form a different hierarchy.

export function HitlSettledRow({ label }: { label: string }) {
  return (
    <Divider icon={<Icon name="check" size="xs" />} intent="accent">
      {label}
    </Divider>
  );
}

import { Divider, Icon } from "@/ui";

export function HitlSettledRow({ label }: { label: string }) {
  return (
    <Divider icon={<Icon name="check" size="xs" />} intent="accent">
      {label}
    </Divider>
  );
}

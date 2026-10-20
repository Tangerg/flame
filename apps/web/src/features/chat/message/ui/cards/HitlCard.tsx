import { Divider } from "@/ui/index";
import { Icon } from "@/ui/index";
export function HitlSettledRow({ label }: { label: string }) {
  return (
    <Divider icon={<Icon name="check" size="xs" />} intent="accent">
      {label}
    </Divider>
  );
}

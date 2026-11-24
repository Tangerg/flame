import { TranscriptMark } from "@/ui/index";
import { Icon } from "@/ui/index";
export function HitlSettledRow({ label }: { label: string }) {
  return (
    <TranscriptMark icon={<Icon name="check" size="xs" />} intent="accent">
      {label}
    </TranscriptMark>
  );
}

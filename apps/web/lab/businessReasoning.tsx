import { ReasoningBlock } from "@/features/chat/message/ui/cards/ReasoningBlock";
import { businessMessages as messages } from "./businessMessages";
import { businessVariant, reasoningVariants } from "./businessVariants";
import { localize, type LabExampleProps } from "./story";

export function ReasoningExample({ variant, locale }: LabExampleProps) {
  const scenario = businessVariant(reasoningVariants, variant);
  return (
    <ReasoningBlock
      status={scenario.status}
      text={localize(
        scenario.short ? messages.samples.reasoningShort : messages.samples.reasoningBody,
        locale,
      )}
    />
  );
}

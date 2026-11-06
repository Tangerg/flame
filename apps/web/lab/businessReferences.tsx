import { MarkdownRenderer } from "@/features/chat/message/ui/markdown/MarkdownRenderer";
import { businessMessages as messages } from "./businessMessages";
import { businessVariant, referenceVariants } from "./businessVariants";
import { localize, type LabExampleProps } from "./story";

export function ReferencesExample({ variant, locale }: LabExampleProps) {
  const scenario = businessVariant(referenceVariants, variant);
  const answer = localize(messages.samples.referenceAnswer, locale);
  const note = localize(
    scenario.long ? messages.samples.referenceLong : messages.samples.referenceShort,
    locale,
  );
  return (
    <MarkdownRenderer
      text={`${answer}\n\n[^来源]: ${note.replaceAll("\n\n", "\n\n    ")}`}
      reveal="instant"
    />
  );
}

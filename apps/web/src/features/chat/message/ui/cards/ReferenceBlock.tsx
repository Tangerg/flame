import * as stylex from "@stylexjs/stylex";
import { useState } from "react";
import { AgentActivityDisclosure } from "@/ui/agent/index";
import { face, leading, type as typeStep } from "@flame/ui/tokens.stylex";
const rb = stylex.create({
  text: {
    maxWidth: "640px",
    whiteSpace: "pre-wrap",
    overflowWrap: "anywhere",
    textAlign: "start",
    lineHeight: leading.prose,
  },
});

// Opening a reference shows exactly what the model received for it.
export function ReferenceBlock({ label, text }: { label: string; text: string }) {
  const [open, setOpen] = useState(false);
  return (
    <AgentActivityDisclosure
      data-message-reference=""
      icon="crosshair"
      shell="line"
      label={label}
      revealClippedLabel
      open={open}
      onOpenChange={setOpen}
      contentClassName={stylex.props(rb.text, face.mono, typeStep.code).className}
    >
      {text}
    </AgentActivityDisclosure>
  );
}

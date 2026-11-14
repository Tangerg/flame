import { useRef } from "react";
import { createRoot } from "react-dom/client";
import { useComposerFileDrop } from "@/features/composer/ui/ComposerFileDrop";
import { type VisualClient } from "./client";

export function mountFileDropReceiver(container: HTMLElement, client: VisualClient) {
  function Receiver() {
    const target = useRef<HTMLDivElement>(null);
    const dragging = useComposerFileDrop(target, true, (files, intake) => intake.addFiles(files));
    return (
      <div
        ref={target}
        data-testid="successor-file-receiver"
        data-dragging={dragging ? "" : undefined}
      >
        Files
      </div>
    );
  }
  const root = createRoot(container);
  root.render(
    <client.Wrapper>
      <Receiver />
    </client.Wrapper>,
  );
  return () => root.unmount();
}

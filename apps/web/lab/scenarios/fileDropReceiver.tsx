import { createRoot } from "react-dom/client";
import { useComposerFileDrop } from "@/features/composer/ui/ComposerFileDrop";
import { type VisualClient } from "./client";

export function mountFileDropReceiver(container: HTMLElement, client: VisualClient) {
  function Receiver({ surfaceKey }: { surfaceKey: string }) {
    const { ref, dragging } = useComposerFileDrop(true, (files, intake) => intake.addFiles(files));
    return (
      <div
        key={surfaceKey}
        ref={ref}
        data-testid="successor-file-receiver"
        data-dragging={dragging ? "" : undefined}
      >
        Files
      </div>
    );
  }
  const root = createRoot(container);
  const render = (surfaceKey: string) =>
    root.render(
      <client.Wrapper>
        <Receiver surfaceKey={surfaceKey} />
      </client.Wrapper>,
    );
  render("original");
  return {
    dispose: () => root.unmount(),
    replaceSurface: () => render("replacement"),
  };
}

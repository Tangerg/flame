import { useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import {
  Button,
  CodeBlock,
  Disclosure,
  FormField,
  ModalDialog,
  Panel,
  PanelHeader,
  TextArea,
  TextField,
  useFieldVisibility,
} from "@flame/ui";
import { createLabEnvironment } from "./environment";
import { disposeOnHmr } from "@/lib/hmr";
import "@flame/ui/styles.css";
import "@/styles/globals.css";

const environment = await createLabEnvironment({
  theme: "light",
  locale: "en",
  motion: "off",
  fontSize: 16,
});
const source = Array.from({ length: 140 }, (_, index) => `const captured${index} = ${index};`).join(
  "\n",
);
function Acceptance() {
  const viewport = useRef<HTMLElement>(null);
  useFieldVisibility(viewport);
  const [panelGrowth, growPanel] = useState(false);
  const [dialogGrowth, growDialog] = useState(false);
  const [open, setOpen] = useState(false);
  const [commands, setCommands] = useState(0);
  return (
    <main
      style={{ height: "100dvh", display: "grid", gridTemplateRows: "auto minmax(0, 1fr)", gap: 8 }}
    >
      <nav aria-label="Native acceptance controls">
        <Button onClick={() => growPanel((value) => !value)}>Grow panel content</Button>
        <Button onClick={() => growDialog((value) => !value)}>Grow dialog content</Button>
        <Button onClick={() => setOpen(true)}>Open native dialog</Button>
      </nav>
      <section ref={viewport} data-native-panel style={{ overflow: "auto", minHeight: 0 }}>
        <Panel
          label="Native form"
          header={<PanelHeader title="Normal-flow fields" />}
          controls={<TextField aria-label="Panel filter" placeholder="Filter" />}
        >
          <p style={{ height: panelGrowth ? 1100 : 120 }}>Captured content before the field.</p>
          <FormField label="Panel note">
            <TextArea rows={4} defaultValue="Keep the exact draft 保留" />
          </FormField>
          <Button onClick={() => setCommands((value) => value + 1)}>Record native command</Button>
          <p role="status">Recorded native commands: {commands}</p>
          <Disclosure title="Native disclosure">
            <TextField aria-label="Disclosure note" defaultValue="Retained disclosure draft" />
          </Disclosure>
          <CodeBlock label="Native code reader" language="typescript">
            {source}
          </CodeBlock>
        </Panel>
      </section>
      <ModalDialog
        open={open}
        onOpenChange={setOpen}
        title="Native field reading"
        closeLabel="Close native dialog"
        footer={<Button onClick={() => setOpen(false)}>Finish reading</Button>}
      >
        <p style={{ height: dialogGrowth ? 1100 : 120, flexShrink: 0 }}>
          Captured dialog content before the field.
        </p>
        <FormField label="Dialog note">
          <TextArea rows={4} defaultValue="Keep the dialog draft 保留" />
        </FormField>
        <FormField label="Autosized note">
          <TextArea autosize defaultValue="Native field sizing" />
        </FormField>
        <CodeBlock label="Dialog code reader" language="typescript">
          {source}
        </CodeBlock>
      </ModalDialog>
    </main>
  );
}
const root = createRoot(document.getElementById("root")!);
root.render(
  <environment.Wrapper>
    <Acceptance />
  </environment.Wrapper>,
);
const dispose = () => {
  root.unmount();
  void environment.dispose();
};
window.addEventListener("pagehide", dispose, { once: true });
disposeOnHmr(dispose);

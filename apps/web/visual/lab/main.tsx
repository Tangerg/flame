import * as stylex from "@stylexjs/stylex";
import { labStyles as styles } from "./labStyles";
import { createRoot } from "react-dom/client";
import { disposeOnHmr } from "@/lib/hmr";
import "@flame/ui/styles.css";
import "@/styles/globals.css";
import "@/styles/overlays.css";
import "./lab.css";
import { catalog } from "./catalog";
import { ComponentLab } from "./ComponentLab";
import { createLabEnvironment } from "./environment";
import { LAB_PATH, readSelection } from "./selection";
import { messages } from "./messages";

const container = document.getElementById("root");
if (!container) throw new Error("Component lab root is missing");
const root = createRoot(container);

try {
  const selection = readSelection(window.location.search, catalog);
  const environment = await createLabEnvironment(selection);
  root.render(
    <environment.Wrapper>
      <ComponentLab initial={selection} environment={environment} />
    </environment.Wrapper>,
  );
  const dispose = () => {
    root.unmount();
    void environment.dispose();
  };
  window.addEventListener("pagehide", dispose, { once: true });
  disposeOnHmr(() => {
    window.removeEventListener("pagehide", dispose);
    dispose();
  });
} catch (error) {
  root.render(
    <main {...stylex.props(styles.error)}>
      <h1>{messages.invalidLink["zh-CN"]}</h1>
      <pre {...stylex.props(styles.errorDetail)}>
        {error instanceof Error ? error.message : String(error)}
      </pre>
      <a href={LAB_PATH}>{messages.restart["zh-CN"]}</a>
    </main>,
  );
}

import * as stylex from "@stylexjs/stylex";
import { FAILURE_TITLE_ID, failurePresentation, failureRootProps, failureStyles } from "./failure";
import { createRoot } from "react-dom/client";
import { disposeOnHmr } from "@/lib/hmr";
import "@flame/ui/styles.css";
import "@/styles/globals.css";
import "@/styles/overlays.css";
import "./lab.css";
import { catalog } from "./catalog";
import { ComponentLab } from "./ComponentLab";
import { createLabEnvironment } from "./environment";
import { readSelection } from "./selection";
import { messages } from "./messages";

const container = document.getElementById("root");
if (!container) throw new Error("Component lab root is missing");
const root = createRoot(container);
const query = new URLSearchParams(window.location.search);

try {
  const selection = readSelection(query.toString(), catalog);
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
  const failure = failurePresentation(query);
  root.render(
    <main
      {...failureRootProps}
      style={failure.style}
      lang={failure.locale}
      aria-labelledby={FAILURE_TITLE_ID}
    >
      <h1 {...stylex.props(failureStyles.heading)} id={FAILURE_TITLE_ID}>
        {messages.invalidLink[failure.locale]}
      </h1>
      <pre {...stylex.props(failureStyles.detail)}>
        {error instanceof Error ? error.message : String(error)}
      </pre>
      <a {...stylex.props(failureStyles.recovery)} href={failure.href}>
        {messages.restart[failure.locale]}
      </a>
    </main>,
  );
}

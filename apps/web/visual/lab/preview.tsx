import * as stylex from "@stylexjs/stylex";
import { previewStyles as styles } from "./previewStyles";
import { useEffect } from "react";
import { createRoot } from "react-dom/client";
import { ErrorBoundary } from "react-error-boundary";
import { disposeOnHmr } from "@/lib/hmr";
import "@flame/ui/styles.css";
import "@/styles/globals.css";
import "@/styles/markdown.css";
import "@/styles/overlays.css";
import "./preview.css";
import { catalog } from "./catalog";
import { createLabEnvironment } from "./environment";
import { installPreviewObservations, OBSERVATION_NAMES } from "./observations";
import { LAB_INSTANCE_QUERY, readSelection } from "./selection";
import { localize } from "./story";
import { messages } from "./messages";

const query = new URLSearchParams(window.location.search);
const observations = installPreviewObservations(window, document, {
  instanceId: query.get(LAB_INSTANCE_QUERY) ?? "standalone",
});
const container = document.getElementById("root");
if (!container) throw new Error("Component preview root is missing");
const root = createRoot(container);

try {
  const selection = readSelection(window.location.search, catalog);
  const story = catalog.story(selection.storyId);
  if (story.kind !== "component")
    throw new Error("Product scenarios require their fixture document");
  const environment = await createLabEnvironment(selection);
  const { default: Example } = await story.load();

  function Ready() {
    useEffect(() => {
      observations.ready();
    }, []);
    return null;
  }

  root.render(
    <environment.Wrapper>
      <ErrorBoundary
        onError={(error) =>
          observations.observe(
            OBSERVATION_NAMES.reactError,
            error instanceof Error ? error.message : String(error),
          )
        }
        fallbackRender={({ error }) => (
          <div {...stylex.props(styles.previewError)} role="alert">
            <h1>{localize(messages.renderError, selection.locale)}</h1>
            <pre {...stylex.props(styles.previewErrorDetail)}>
              {error instanceof Error ? error.message : String(error)}
            </pre>
          </div>
        )}
      >
        <main
          {...stylex.props(styles.example)}
          aria-label={localize(story.title, selection.locale)}
        >
          <Example
            variant={selection.variantId}
            locale={selection.locale}
            observe={observations.observe}
          />
        </main>
        <Ready />
      </ErrorBoundary>
    </environment.Wrapper>,
  );
  const dispose = () => {
    observations.dispose();
    root.unmount();
    void environment.dispose();
  };
  window.addEventListener("pagehide", dispose, { once: true });
  disposeOnHmr(() => {
    window.removeEventListener("pagehide", dispose);
    dispose();
  });
} catch (error) {
  const reason = error instanceof Error ? error.message : String(error);
  observations.observe(OBSERVATION_NAMES.reactError, reason);
  root.render(
    <main {...stylex.props(styles.previewError)} role="alert">
      <h1>{messages.renderError["zh-CN"]}</h1>
      <pre {...stylex.props(styles.previewErrorDetail)}>{reason}</pre>
    </main>,
  );
}

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
import { LAB_INSTANCE_QUERY, readSelection, recoveryURL } from "./selection";
import { localize } from "./story";
import { messages } from "./messages";
import { FAILURE_TITLE_ID, failurePresentation, failureRootProps, failureStyles } from "./failure";

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
          <main
            {...failureRootProps}
            role="alert"
            lang={selection.locale}
            aria-labelledby={FAILURE_TITLE_ID}
          >
            <h1 {...stylex.props(failureStyles.heading)} id={FAILURE_TITLE_ID}>
              {localize(messages.renderError, selection.locale)}
            </h1>
            <pre {...stylex.props(failureStyles.detail)}>
              {error instanceof Error ? error.message : String(error)}
            </pre>
            <a {...stylex.props(failureStyles.recovery)} href={recoveryURL(selection)}>
              {localize(messages.restart, selection.locale)}
            </a>
          </main>
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
  const failure = failurePresentation(query);
  root.render(
    <main
      {...failureRootProps}
      style={failure.style}
      role="alert"
      lang={failure.locale}
      aria-labelledby={FAILURE_TITLE_ID}
    >
      <h1 {...stylex.props(failureStyles.heading)} id={FAILURE_TITLE_ID}>
        {messages.renderError[failure.locale]}
      </h1>
      <pre {...stylex.props(failureStyles.detail)}>{reason}</pre>
      <a {...stylex.props(failureStyles.recovery)} href={failure.href}>
        {messages.restart[failure.locale]}
      </a>
    </main>,
  );
}

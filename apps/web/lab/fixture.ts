import { installPreviewObservations, OBSERVATION_NAMES } from "./observations";
import { LAB_INSTANCE_QUERY } from "./selection";
import { messages } from "./messages";
import * as stylex from "@stylexjs/stylex";
import { FAILURE_TITLE_ID, failurePresentation, failureRootProps, failureStyles } from "./failure";

const query = new URLSearchParams(window.location.search);
const observations = installPreviewObservations(window, document, {
  instanceId: query.get(LAB_INSTANCE_QUERY) ?? "standalone",
});

try {
  await import("./scenarios/fixtureRuntime");
  const ready = () => {
    if (!document.documentElement.hasAttribute("data-visual-ready")) return;
    observer.disconnect();
    observations.ready();
  };
  const observer = new MutationObserver(ready);
  observer.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ["data-visual-ready"],
  });
  window.addEventListener("pagehide", () => observer.disconnect(), { once: true });
  import.meta.hot?.dispose(() => observer.disconnect());
  ready();
} catch (error) {
  const reason = error instanceof Error ? error.message : String(error);
  observations.observe(OBSERVATION_NAMES.reactError, reason);
  const root = document.getElementById("root");
  if (root) {
    // Preserve normal fixture CSS ordering; only failed admission needs this fallback load.
    await Promise.all([import("@flame/ui/styles.css"), import("@/styles/globals.css")]);
    const failure = failurePresentation(query, "fixture");
    const reader = document.createElement("main");
    reader.className = failureRootProps.className ?? "";
    for (const [name, value] of Object.entries(failure.variables))
      reader.style.setProperty(name, value);
    reader.style.colorScheme = failure.scheme;
    reader.lang = failure.locale;
    reader.setAttribute("role", "alert");
    reader.setAttribute("aria-labelledby", FAILURE_TITLE_ID);
    const title = document.createElement("h1");
    title.id = FAILURE_TITLE_ID;
    title.className = stylex.props(failureStyles.heading).className ?? "";
    title.textContent = messages.renderError[failure.locale];
    const detail = document.createElement("pre");
    detail.className = stylex.props(failureStyles.detail).className ?? "";
    detail.textContent = reason;
    const recovery = document.createElement("a");
    recovery.className = stylex.props(failureStyles.recovery).className ?? "";
    recovery.href = failure.href;
    recovery.textContent = messages.restart[failure.locale];
    reader.append(title, detail, recovery);
    root.replaceChildren(reader);
  }
}

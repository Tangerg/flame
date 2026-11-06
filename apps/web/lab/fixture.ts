import { installPreviewObservations, OBSERVATION_NAMES } from "./observations";
import { LAB_INSTANCE_QUERY } from "./selection";
import { messages } from "./messages";

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
    const title = document.createElement("h1");
    title.textContent = messages.renderError[query.get("locale") === "en" ? "en" : "zh-CN"];
    const detail = document.createElement("pre");
    detail.textContent = reason;
    root.replaceChildren(title, detail);
    root.setAttribute("role", "alert");
  }
}

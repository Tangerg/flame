import { installPreviewObservations, OBSERVATION_NAMES } from "./observations";
import { LAB_INSTANCE_QUERY } from "./selection";
import { messages } from "./messages";

const query = new URLSearchParams(window.location.search);
const observations = installPreviewObservations(window, document, {
  instanceId: query.get(LAB_INSTANCE_QUERY) ?? "standalone",
});

try {
  // The existing entry remains the only owner of protocol fixtures and their teardown.
  await import("../main");
  observations.ready();
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

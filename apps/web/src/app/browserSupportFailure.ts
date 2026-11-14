import { browserTargets } from "@flame/component-sdk/browser-policy";
import { initialLocale, localeToBcp47 } from "@/lib/i18n/locale";
import { failureMessage } from "@/lib/diagnostics";
import { startupCopy } from "./startupCopy.generated";

export type BrowserStartupFailure =
  { status: "unsupported"; missing: readonly string[] } | { status: "failed"; cause: unknown };

function browserDescription(target: string): string {
  const name = target.match(/^[a-z]+/)![0];
  return `${name.charAt(0).toUpperCase()}${name.slice(1)} ≥${target.slice(name.length)}`;
}

export function showBrowserStartupFailure(root: HTMLElement, failure: BrowserStartupFailure): void {
  const requested = initialLocale();
  const locale = Object.prototype.hasOwnProperty.call(startupCopy, requested)
    ? (requested as keyof typeof startupCopy)
    : "en";
  const copy = startupCopy[locale];
  const document = root.ownerDocument;
  document.documentElement.lang = localeToBcp47(locale);
  const main = document.createElement("main");
  main.dataset.slot = "startup-failure";
  main.setAttribute("role", "alert");
  main.tabIndex = 0;
  main.setAttribute("aria-label", copy["startup.failed.title"]);
  const content = document.createElement("div");
  const heading = document.createElement("h1");
  heading.textContent = copy["startup.failed.title"];
  const body = document.createElement("p");
  body.textContent =
    copy[failure.status === "unsupported" ? "startup.unsupported.body" : "startup.loadFailed.body"];
  content.append(heading, body);

  if (failure.status === "unsupported") {
    const requirements = document.createElement("dl");
    for (const [label, values] of [
      [copy["startup.unsupported.browsers"], browserTargets.map(browserDescription)],
      [copy["startup.unsupported.features"], failure.missing],
    ] as const) {
      const term = document.createElement("dt");
      term.textContent = label;
      const definition = document.createElement("dd");
      const list = document.createElement("ul");
      for (const value of values) {
        const item = document.createElement("li");
        item.textContent = value;
        list.append(item);
      }
      definition.append(list);
      requirements.append(term, definition);
    }
    content.append(requirements);
  } else {
    const details = document.createElement("details");
    details.open = true;
    const summary = document.createElement("summary");
    summary.textContent = copy["plugins.renderFailed.details"];
    const diagnostic = document.createElement("pre");
    diagnostic.tabIndex = 0;
    diagnostic.setAttribute("role", "region");
    diagnostic.setAttribute("aria-label", copy["plugins.renderFailed.details"]);
    diagnostic.textContent = failureMessage(failure.cause);
    details.append(summary, diagnostic);
    const retry = document.createElement("button");
    retry.type = "button";
    retry.textContent = copy["common.retry"];
    retry.addEventListener("click", () => document.defaultView?.location.reload());
    content.append(details, retry);
  }

  main.append(content);
  root.replaceChildren(main);
}

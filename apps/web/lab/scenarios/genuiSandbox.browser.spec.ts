import { expect, test, type Page } from "@playwright/test";
import { rolldown } from "rolldown";
import { resolve } from "node:path";

const origin = "http://127.0.0.1:4175";
const documentURL = `${origin}/lab/`;

async function openHost(page: Page) {
  const response = await page.goto(documentURL);
  expect(response?.status()).toBe(200);
  const headers = response!.headers();
  expect(headers["content-security-policy"]).toContain("'strict-dynamic'");
  expect(headers["content-security-policy"]).toContain("frame-ancestors 'none'");
  expect(headers["x-frame-options"]).toBe("DENY");
  expect(headers["cache-control"]).toBe("no-store");
  await expect(page.locator("#root")).toBeVisible();
}

async function showProgram(page: Page, title: string, html: string) {
  await page.evaluate(
    ({ title, html }) => {
      const frame = document.createElement("iframe");
      frame.title = "Security program wrapper";
      frame.sandbox.add("allow-scripts");
      frame.src = "/genui-sandbox.html";
      frame.addEventListener("load", () => {
        const channel = new MessageChannel();
        const id = crypto.randomUUID();
        channel.port1.onmessage = (event) => {
          if (event.data?.type !== "flame-genui-ready" || event.data.id !== id) return;
          channel.port1.onmessage = null;
          channel.port1.postMessage({ type: "flame-genui-content", id, title, html });
        };
        frame.contentWindow!.postMessage({ type: "flame-genui-connect", id }, "*", [channel.port2]);
      });
      document.body.append(frame);
    },
    { title, html },
  );
  return page.frameLocator('iframe[title="Security program wrapper"]').frameLocator("iframe");
}

test("the application frame policy permits the exact privacy-enhanced YouTube origin", async ({
  page,
}) => {
  const requests: string[] = [];
  await page.route("https://www.youtube-nocookie.com/**", async (route) => {
    requests.push(route.request().url());
    await route.fulfill({ contentType: "text/html", body: "<p>Original media frame</p>" });
  });
  await page.route("https://genui-probe.invalid/**", async (route) => {
    requests.push(route.request().url());
    await route.fulfill({ body: "unexpected frame" });
  });
  await openHost(page);
  await page.evaluate(() => {
    for (const [title, src] of [
      ["Allowed media", "https://www.youtube-nocookie.com/embed/abcdefghijk"],
      ["Other origin", "https://genui-probe.invalid/frame"],
    ]) {
      const frame = document.createElement("iframe");
      frame.title = title!;
      frame.src = src!;
      document.body.append(frame);
    }
  });
  await expect(
    page.frameLocator('iframe[title="Allowed media"]').getByText("Original media frame"),
  ).toBeVisible();
  expect(requests).toEqual(["https://www.youtube-nocookie.com/embed/abcdefghijk"]);
});

test("sandbox runs local programs and confines scripts, requests and navigation", async ({
  page,
}) => {
  const escapedRequests: string[] = [];
  await page.route("https://genui-probe.invalid/**", async (route) => {
    escapedRequests.push(route.request().url());
    await route.fulfill({ body: "escaped" });
  });
  await openHost(page);
  const program = await showProgram(
    page,
    "Local program",
    `<button id="add">Add</button><output id="count">0</output>
<button id="navigate">Navigate</button><output id="isolation"></output>
<script>
let count = 0;
document.querySelector('#add').onclick = () => document.querySelector('#count').textContent = String(++count);
try { parent.document.body; document.querySelector('#isolation').textContent = 'escaped'; }
catch { document.querySelector('#isolation').textContent = 'isolated'; }
fetch('https://genui-probe.invalid/fetch').catch(() => {});
new Image().src = 'https://genui-probe.invalid/image';
document.querySelector('#navigate').onclick = () => { location.href = 'https://genui-probe.invalid/navigation'; };
</script>`,
  );
  await expect(program.locator("#isolation")).toHaveText("isolated");
  await program.getByRole("button", { name: "Add", exact: true }).click();
  await expect(program.locator("#count")).toHaveText("1");
  await program.getByRole("button", { name: "Navigate", exact: true }).click();
  await expect(page.locator("#root")).toBeVisible();
  await expect.poll(() => escapedRequests).toEqual([]);
  expect(page.frames().every((frame) => !frame.url().includes("genui-probe.invalid"))).toBe(true);
});

test("authored markup cannot close the trusted wrapper or navigate its parent", async ({
  page,
}) => {
  await openHost(page);
  const program = await showProgram(
    page,
    '" onload="parent.document.body.remove()',
    `</iframe><script>try { top.document.body.remove(); } catch {}
try { parent.location.href = 'https://genui-probe.invalid/parent'; } catch {}</script><p>Program loaded</p>`,
  );
  await expect(program.getByText("Program loaded")).toBeVisible();
  await expect(page.locator("#root")).toBeVisible();
  await expect(page.locator('iframe[title="Security program wrapper"]')).toHaveCount(1);
  await expect(
    page.frameLocator('iframe[title="Security program wrapper"]').locator("iframe"),
  ).toHaveCount(1);
});

test("nonce delivery blocks injected parser scripts and never revalidates an old nonce body", async ({
  page,
  request,
}) => {
  await page.addInitScript(() => {
    const directives: string[] = [];
    Reflect.set(window, "__cspProbeDirectives", directives);
    document.addEventListener("securitypolicyviolation", (event) =>
      directives.push(event.effectiveDirective),
    );
  });
  await page.route(documentURL, async (route) => {
    const response = await route.fetch();
    const body = (await response.text()).replace(
      "</head>",
      "<script>window.__untrustedInlineRan = true</script></head>",
    );
    await route.fulfill({ response, body });
  });
  await openHost(page);
  await expect
    .poll(() => page.evaluate(() => Reflect.get(window, "__cspProbeDirectives")))
    .toContain("script-src-elem");
  expect(await page.evaluate(() => "__untrustedInlineRan" in window)).toBe(false);
  const previous = await request.get(documentURL);
  const current = await request.get(documentURL, {
    headers: { "If-None-Match": "*", "If-Modified-Since": "Wed, 31 Dec 2099 23:59:59 GMT" },
  });
  expect(current.status()).toBe(200);
  expect(current.headers()["content-security-policy"]).not.toBe(
    previous.headers()["content-security-policy"],
  );
  const [, nonce] = current.headers()["content-security-policy"]!.match(/'nonce-([^']+)'/u)!;
  expect(await current.text()).toContain(`nonce="${nonce}"`);
});

test("the main application refuses same-origin framing without blocking the program response", async ({
  page,
}) => {
  await openHost(page);
  const embedded = await page.evaluate(
    (url) =>
      new Promise<boolean>((done) => {
        const frame = document.createElement("iframe");
        frame.title = "Embedded application";
        frame.onload = () => {
          try {
            done(
              frame.contentDocument?.querySelector("#root") !== null &&
                frame.contentDocument !== null,
            );
          } catch {
            done(false);
          }
        };
        frame.src = url;
        document.body.append(frame);
      }),
    documentURL,
  );
  expect(embedded).toBe(false);
  const program = await showProgram(page, "Allowed local program", "<p>Independent program</p>");
  await expect(program.getByText("Independent program")).toBeVisible();
});

test("the trusted Frame loader verifies captured module bytes before data-module evaluation under CSP", async ({
  page,
}) => {
  const entry = resolve(import.meta.dirname, "frame-csp-probe.js");
  const bundle = await rolldown({
    input: entry,
    plugins: [
      {
        name: "frame-csp-probe",
        resolveId(id) {
          if (id === entry) return id;
        },
        load(id) {
          if (id !== entry) return;
          return `import { Lifetime } from '@frame/sdk';
import { RealmModules } from '@frame/sdk/browser';
const bytes = new TextEncoder().encode("import { createElement } from 'react'; export const captured = typeof createElement === 'function' ? 42 : 0;");
const digest = Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256', bytes)), value => value.toString(16).padStart(2, '0')).join('');
const plan = { authority: { id: 'csp-probe', epoch: 1 } };
const unit = { instance: { source: { installationId: 'csp-probe', release: 'a'.repeat(64) } }, module: { path: 'component.js', digest } };
const owner = new Lifetime('csp-probe');
const modules = new RealmModules(async () => ({ bytes, close: async () => ({ status: 'released' }) }));
const accepted = await modules.evaluate(unit, owner, plan);
let refusal;
try {
  await new RealmModules(async () => ({ bytes: new TextEncoder().encode('window.__unverifiedModuleRan = true'), close: async () => ({ status: 'released' }) })).evaluate(unit, owner, plan);
} catch (error) { refusal = error.code; }
window.__frameCSP = { value: accepted.captured, refusal, unverifiedRan: '__unverifiedModuleRan' in window };
owner.beginClose();`;
        },
      },
    ],
  });
  let code: string;
  try {
    const output = await bundle.generate({ format: "esm", codeSplitting: false });
    const chunk = output.output.find((file) => file.type === "chunk" && file.isEntry);
    if (!chunk || chunk.type !== "chunk") throw new Error("Frame CSP probe was not emitted");
    code = chunk.code;
  } finally {
    await bundle.close();
  }
  await page.route("**/frame-csp-probe.js", (route) =>
    route.fulfill({ contentType: "text/javascript", body: code }),
  );
  await openHost(page);
  await page.evaluate(() => {
    const script = document.createElement("script");
    script.type = "module";
    script.nonce = document.querySelector<HTMLMetaElement>('meta[property="csp-nonce"]')!.nonce;
    script.src = "/frame-csp-probe.js";
    document.head.append(script);
  });
  await expect
    .poll(() => page.evaluate(() => (window as unknown as { __frameCSP?: unknown }).__frameCSP))
    .toEqual({ value: 42, refusal: "contract_mismatch", unverifiedRan: false });
});

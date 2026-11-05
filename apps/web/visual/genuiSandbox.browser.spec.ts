import { expect, test } from "@playwright/test";
import { sandboxDocument } from "../../../plugins/genui/component/sandbox";
import { readFileSync } from "node:fs";

test("the application frame policy permits the exact privacy-enhanced YouTube origin", async ({
  page,
}) => {
  const template = readFileSync(new URL("../index.template.html", import.meta.url), "utf8");
  const policy = template.match(
    /<meta\s+http-equiv="Content-Security-Policy"\s+content="([^"]+)"\s*\/>/u,
  )?.[1];
  expect(policy).toBeDefined();
  const requests: string[] = [];
  await page.route("https://www.youtube-nocookie.com/**", async (route) => {
    requests.push(route.request().url());
    await route.fulfill({ contentType: "text/html", body: "<p>Original media frame</p>" });
  });
  await page.route("https://genui-probe.invalid/**", async (route) => {
    requests.push(route.request().url());
    await route.fulfill({ body: "unexpected frame" });
  });
  await page.setContent(
    `<meta http-equiv="Content-Security-Policy" content="${policy}"><iframe title="Allowed media" src="https://www.youtube-nocookie.com/embed/abcdefghijk"></iframe><iframe title="Other origin" src="https://genui-probe.invalid/frame"></iframe>`,
  );
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
  const source = sandboxDocument(
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
  await page.setContent('<main id="host">Host</main>');
  await page.evaluate((srcdoc) => {
    const iframe = document.createElement("iframe");
    iframe.sandbox.add("allow-scripts");
    iframe.srcdoc = srcdoc;
    document.body.append(iframe);
  }, source);
  const program = page.frameLocator("iframe").frameLocator("iframe");
  await expect(program.locator("#isolation")).toHaveText("isolated");
  await program.getByRole("button", { name: "Add", exact: true }).click();
  await expect(program.locator("#count")).toHaveText("1");
  await program.getByRole("button", { name: "Navigate", exact: true }).click();
  await expect(page.getByText("Host", { exact: true })).toBeVisible();
  await expect.poll(() => escapedRequests).toEqual([]);
  expect(page.frames().every((frame) => !frame.url().includes("genui-probe.invalid"))).toBe(true);
});

test("authored markup cannot close the trusted wrapper or navigate its parent", async ({
  page,
}) => {
  await page.setContent('<main id="host">Host</main>');
  const source = sandboxDocument(
    '" onload="parent.document.body.remove()',
    `</iframe><script>try { top.document.querySelector('#host').remove(); } catch {}
try { parent.location.href = 'https://genui-probe.invalid/parent'; } catch {}</script><p>Program loaded</p>`,
  );
  await page.evaluate((srcdoc) => {
    const iframe = document.createElement("iframe");
    iframe.sandbox.add("allow-scripts");
    iframe.srcdoc = srcdoc;
    document.body.append(iframe);
  }, source);
  await expect(
    page.frameLocator("iframe").frameLocator("iframe").getByText("Program loaded"),
  ).toBeVisible();
  await expect(page.getByText("Host", { exact: true })).toBeVisible();
  expect(page.frames()).toHaveLength(3);
});

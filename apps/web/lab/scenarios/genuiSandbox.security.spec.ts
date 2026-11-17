import { expect, test } from "@playwright/test";
test.setTimeout(30_000);

test("production CSP permits the built A2UI plugin and Pierre JS workers while refusing dynamic code and Wasm", async ({
  page,
}) => {
  const response = await page.goto("/lab/a2ui.html");
  const policy = response!.headers()["content-security-policy"]!;
  expect(policy).toContain("'strict-dynamic'");
  expect(policy).not.toContain("'unsafe-eval'");
  expect(policy).not.toContain("'wasm-unsafe-eval'");
  await page.getByRole("button", { name: "Show interface", exact: true }).click();
  await expect(page.getByRole("textbox", { name: "Review note", exact: true })).toHaveValue(
    "original",
  );
  await page.getByRole("button", { name: "Probe Highlights", exact: true }).click();
  const probes = page.getByLabel("Highlights CSP probes");
  await expect(probes).toContainText(/blocked:.*WebAssembly/s);
  await expect(probes).not.toContainText("unexpected success");
  expect((await probes.textContent())?.match(/blocked:/g)).toHaveLength(2);
  await page.getByRole("button", { name: "Render code", exact: true }).click();
  await expect(page.locator("diffs-container").locator("[data-line]").first()).toBeVisible();
  const blocked = await page.evaluate(async () => {
    let main = false;
    try {
      await WebAssembly.compile(new Uint8Array([0, 97, 115, 109, 1, 0, 0, 0]));
    } catch {
      main = true;
    }
    const url = URL.createObjectURL(
      new Blob(
        [
          `onmessage=async()=>{try{await WebAssembly.compile(new Uint8Array([0,97,115,109,1,0,0,0]));postMessage(false)}catch{postMessage(true)}}`,
        ],
        { type: "text/javascript" },
      ),
    );
    const worker = new Worker(url);
    const child = await new Promise<boolean>((resolve, reject) => {
      worker.onmessage = (event) => resolve(event.data === true);
      worker.onerror = reject;
      worker.postMessage(null);
    });
    worker.terminate();
    URL.revokeObjectURL(url);
    return { main, worker: child };
  });
  expect(blocked).toEqual({ main: true, worker: true });
});

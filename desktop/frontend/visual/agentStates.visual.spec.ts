import { expect, test, type Locator, type Page } from "./test";
import { freezeVisualClock } from "./frozenClock";
import { VISUAL_AGENT_STATES, type VisualAgentState } from "./agentSessionSnapshots";
import { VISUAL_CONTEXT_TOKENS, VISUAL_PRIMARY_MODEL_CONTEXT_WINDOW } from "./agentFixtureFacts";
import { contextUsageReadout } from "@/plugins/builtin/chat/context-usage/application/contextUsageReadout";
import { fmtTokens } from "@/lib/format";

const EXPECTED_ATTENTION: Record<VisualAgentState, string> = {
  empty: "idle",
  idle: "finished",
  running: "running",
  "answer-opening": "running",
  steer: "running",
  waiting: "waiting",
  question: "waiting",
  terminal: "finished",
  canceled: "finished",
  error: "finished",
  "error-retryable": "finished",
  recovery: "finished",
  "cwd-missing": "finished",
  delegated: "running",
  "long-content": "finished",
  narrative: "finished",
  "tool-shells": "finished",
  "tool-search": "finished",
  "tool-remote": "finished",
  "tool-agentic": "finished",
  "tool-tail": "finished",
  "question-multi": "waiting",
  waves: "running",
};

const GPT_5_6_SOL_CAPABILITY_NAME =
  "GPT-5.6 Sol 1.1M context · text + image + pdf input · Reasoning none / low / medium / high / xhigh / max · 922k max input · 128k max output · text output · Tools · Structured output · Knowledge 2026-02-16T00:00:00Z";
const QWEN_MT_PLUS_CAPABILITY_NAME =
  "Qwen MT Plus Alibaba 32.8k context · text input · text output";

test("every declared state carries an expected attention", () => {
  expect(Object.keys(EXPECTED_ATTENTION).sort()).toEqual([...VISUAL_AGENT_STATES].sort());
});

for (const state of VISUAL_AGENT_STATES) {
  test(`canonical agent projection renders ${state}`, async ({ page }) => {
    await page.goto(`/visual/?fixture=agent&theme=light&state=${state}`);
    await page.locator("html[data-visual-ready]").waitFor();

    const fixture = page.getByTestId("agent-state");
    await expect(fixture).toHaveAttribute("data-state", state);
    await expect(fixture).toHaveAttribute("data-attention", EXPECTED_ATTENTION[state]);
  });
}

test("HITL approval settles through the exact Run and Item identity", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=waiting");
  await page.locator("html[data-visual-ready]").waitFor();

  await page.getByRole("button", { name: /Allow once/ }).click();

  await expect(page.getByText("Approved", { exact: true })).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("data-visual-resumed-run", "run_root");
  await expect(page.locator("html")).toHaveAttribute("data-visual-resumed-item", "item_approval");
});

test("a pending approval uses the Codex neutral request surface", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=waiting");
  await page.locator("html[data-visual-ready]").waitFor();

  const surface = page.locator('[data-slot="approval-surface"]');
  await expect(surface).toHaveCSS("border-top-width", "0px");
  await expect(surface).toHaveCSS("border-radius", "20px");
  await expect(surface.getByText("Shell", { exact: true })).toBeVisible();
  await expect(
    page.getByText("Run the race detector across the workspace before committing.", {
      exact: true,
    }),
  ).toBeVisible();
  await expect(page.getByText("Approval required", { exact: true })).toHaveCount(0);
  await expect(page.getByText("Medium risk", { exact: true })).toHaveCount(0);
  await expect(page.getByText("go test -race ./...", { exact: true })).toHaveCount(1);
  await expect(page.getByText("Run the race detector", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("checkbox")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Approval options" })).toBeVisible();

  await page.getByRole("button", { name: /Allow once/ }).click();
  await expect(surface).toHaveCount(0);
});

test("HITL rejection preserves the same exact interrupt identity", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=waiting");
  await page.locator("html[data-visual-ready]").waitFor();

  await page.getByRole("button", { name: /Deny/ }).click();

  await expect(page.getByText("Declined", { exact: true })).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("data-visual-resumed-run", "run_root");
  await expect(page.locator("html")).toHaveAttribute("data-visual-resumed-item", "item_approval");
});

test("question settlement uses the exact interrupt identity", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=question");
  await page.locator("html[data-visual-ready]").waitFor();

  const request = page.locator('[data-slot="question-request-surface"]');
  await expect(request).toBeVisible();
  await expect(request).toHaveCSS("border-radius", "20px");
  await expect(request).toHaveCSS("border-top-width", "0px");
  await expect(page.locator('[data-slot="composer-root"]')).toHaveCount(0);
  await expect(page.getByText("Input needed", { exact: true })).toHaveCount(0);
  await expect(page.getByText("Gate", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("radio", { name: /Race detector/ })).toHaveAttribute(
    "aria-checked",
    "true",
  );

  await page.getByRole("radio", { name: /Race detector/ }).click();
  await page
    .getByRole("textbox", { name: "What should this gate protect?" })
    .fill("Runtime boundaries and cancellation paths.");
  await page.getByRole("button", { name: "Next", exact: true }).click();

  const settled = page.getByRole("button", { name: "Asked 2 questions" });
  await expect(settled).toBeVisible();
  await expect(settled).toHaveAttribute("aria-expanded", "false");
  await expect(page.getByText("What should this gate protect?", { exact: true })).toHaveCount(0);
  await expect(settled.locator("xpath=../..")).toHaveScreenshot(
    "question-settled-collapsed-light.png",
  );
  await settled.click();
  await expect(page.getByText("What should this gate protect?", { exact: true })).toBeVisible();
  await expect(page.getByText("Runtime boundaries and cancellation paths.")).toBeVisible();
  await expect(page.locator("[data-settled-answer]").first()).toHaveCSS("white-space", "pre-wrap");
  await expect(settled.locator("xpath=../..")).toHaveScreenshot(
    "question-settled-expanded-light.png",
  );
  await expect(page.locator("html")).toHaveAttribute("data-visual-resumed-run", "run_root");
  await expect(page.locator("html")).toHaveAttribute("data-visual-resumed-item", "item_question");
  await expect(page.locator("html")).toHaveAttribute(
    "data-visual-resumed-response",
    JSON.stringify({
      type: "answer",
      answers: [["Race detector"], ["Runtime boundaries and cancellation paths."]],
    }),
  );
});

test("question skip sends real ordered empty answers and restores the composer", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=question");
  await page.locator("html[data-visual-ready]").waitFor();

  await page.getByRole("button", { name: "Skip", exact: true }).click();
  await expect(page.getByRole("textbox", { name: "What should this gate protect?" })).toBeVisible();
  await page.getByRole("button", { name: "Skip", exact: true }).click();

  await expect(page.locator("html")).toHaveAttribute(
    "data-visual-resumed-response",
    JSON.stringify({ type: "answer", answers: [[], []] }),
  );
  await expect(page.locator('[data-slot="question-request-surface"]')).toHaveCount(0);
  await expect(page.locator('[data-slot="composer-root"]')).toBeVisible();
});

test("question choices keep their descriptions inline without a comparison sidecar", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=question");
  await page.locator("html[data-visual-ready]").waitFor();

  await expect(page.getByRole("radio", { name: /Race detector/ })).toContainText(
    "Exercise concurrency and cancellation paths.",
  );
  await expect(page.getByRole("region", { name: "Race detector" })).toHaveCount(0);
  await expect(page.getByText("go test -race ./...")).toHaveCount(0);
  await expect(page.getByText("npm run test:visual")).toHaveCount(0);
  await expect(page).toHaveScreenshot("agent-light-question-preview.png");
});

test("delegated cancellation targets the selected child Run", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=delegated");
  await page.locator("html[data-visual-ready]").waitFor();

  await page.getByRole("button", { name: "Cancel this run" }).first().click();

  await expect(page.locator("html")).toHaveAttribute("data-visual-canceled-run", "run_child");
});

test("delegated narrative stays under its exact spawning Item anchor", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=delegated");
  await page.locator("html[data-visual-ready]").waitFor();

  const spawningItem = page.locator("#item_delegate");
  await expect(spawningItem).toHaveCount(1);
  await expect(spawningItem.getByRole("button", { name: /Sub-agent/ }).first()).toBeVisible();
});

test("a delegated sub-agent reads as a nested line, not a card", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=delegated");
  await page.locator("html[data-visual-ready]").waitFor();

  const rows = page.getByRole("button", { name: /Sub-agent/ });
  await expect(rows).toHaveCount(6);

  await expect(rows.nth(0)).toContainText("Needs input");
  await expect(rows.nth(1)).toContainText("Running");
  const nesting = await rows.nth(1).evaluate((deep, shallowId) => {
    const shallow = document.getElementById(shallowId);
    return shallow ? shallow.contains(deep) : null;
  }, "item_delegate");
  expect(nesting).toBe(true);

  const shell = await rows.nth(0).evaluate((row) => {
    const style = getComputedStyle(row);
    return { background: style.backgroundColor, radius: style.borderTopLeftRadius };
  });
  expect(shell.background).toBe("rgba(0, 0, 0, 0)");
});

test("a sub-agent's own delegation indents one step further, from both sides", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=delegated");
  await page.locator("html[data-visual-ready]").waitFor();

  await page.locator("[aria-expanded='false']").filter({ hasText: "Sub-agent" }).first().click();
  await expect(page.getByText("Package graph verification is still running.")).toBeVisible();

  const boxes = await page.evaluate(() =>
    Object.fromEntries(
      [...document.querySelectorAll("[data-block-anchor]")].map((element) => [
        element.getAttribute("data-block-anchor"),
        {
          x: Math.round(element.getBoundingClientRect().x),
          width: Math.round(element.getBoundingClientRect().width),
        },
      ]),
    ),
  );

  const root = boxes["turn:item_response:b:0"]!;
  const child = boxes["turn:item_child_response:b:0"]!;
  const grandchild = boxes["turn:item_nested_response:b:0"]!;
  const step = child.x - root.x;
  expect(step).toBeGreaterThan(0);
  expect(grandchild.x - child.x).toBe(step);
  expect(root.width - child.width).toBe(step * 2);
  expect(child.width - grandchild.width).toBe(step * 2);
});

test("running composer exposes both steer and stop actions without unnamed controls", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=steer");
  await page.locator("html[data-visual-ready]").waitFor();

  await expect(page.getByRole("button", { name: "Steer the running turn" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Stop" })).toBeVisible();

  await page.getByRole("button", { name: "Stop" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-visual-stopped-root", "run_root");

  await page.getByRole("button", { name: "Steer the running turn" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-visual-steered-run", "run_root");
  await expect(page.locator("html")).toHaveAttribute("data-visual-steered-segment", "seg_root");
  await expect(page.locator("html")).toHaveAttribute(
    "data-visual-sent-input",
    /Tighten the error copy and continue/,
  );
  await expect(page.getByRole("textbox", { name: "Message composer" })).toHaveValue("");
});

test("a running Goal exposes Pause while the active turn exposes Stop", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=running");
  await page.locator("html[data-visual-ready]").waitFor();

  await expect(page.getByRole("button", { name: "Clear goal", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Pause goal", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Edit goal", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Stop", exact: true })).toHaveCount(1);
});

for (const theme of ["light", "dark"] as const) {
  test(`Goal is a compact composer mode without duplicate fields ${theme}`, async ({ page }) => {
    await page.goto(`/visual/?fixture=agent&theme=${theme}&state=idle`);
    await page.locator("html[data-visual-ready]").waitFor();

    const composerFooter = page.locator('[data-slot="composer-footer"]');
    const input = page.getByRole("textbox", { name: "Message composer" });
    await input.fill("/goal");
    await input.press("Enter");

    const mode = page.getByRole("button", { name: "Exit Goal mode" });
    await expect(mode).toBeVisible();
    await expect(mode).toHaveAttribute("aria-pressed", "true");
    await expect(input).toHaveValue("");
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(page.getByRole("spinbutton")).toHaveCount(0);
    await expect(composerFooter).toHaveScreenshot(`goal-composer-mode-${theme}.png`);

    await mode.click();
    await expect(mode).toHaveCount(0);
  });
}

for (const theme of ["light", "dark"] as const) {
  test(`the standing Goal opens the compact objective editor ${theme}`, async ({ page }) => {
    await page.goto(`/visual/?fixture=agent&theme=${theme}&state=running`);
    await page.locator("html[data-visual-ready]").waitFor();

    await page.getByRole("button", { name: "Edit goal", exact: true }).click();

    const dialog = page.getByRole("dialog", { name: "Edit goal" });
    const backdrop = page.locator('[data-slot="text-editor-backdrop"]');
    const objective = dialog.getByRole("textbox", { name: "Goal" });
    await expect(dialog).toBeVisible();
    await expect(backdrop).toHaveCSS("background-color", "rgba(0, 0, 0, 0.133)");
    await expect(objective).toHaveValue(
      "Get the desktop suite green on Linux without loosening any gate or skipping a test",
    );
    await expect(objective).toHaveAttribute("rows", "12");
    await expect(dialog.getByRole("button", { name: "Save" })).toBeDisabled();
    await expect(dialog).toHaveScreenshot(`goal-editor-${theme}.png`);
    await dialog.getByRole("button", { name: "Cancel" }).click();
    await expect(dialog).not.toBeVisible();
  });
}

test("the compact Plan pill reveals the production checklist on hover", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=running");
  await page.locator("html[data-visual-ready]").waitFor();

  const plan = page.getByRole("button", { name: "Step 2 / 3" });
  await expectStableBox(plan);
  await plan.hover();

  await expect(page.getByText("Run quality gates", { exact: true })).toBeVisible();
  const tooltip = page.getByRole("tooltip");
  await expect(tooltip).toBeVisible();
  await expect(tooltip).toHaveScreenshot("active-plan-tooltip-light.png");
});

test("the active plan stays with the composer instead of claiming the transcript header", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=running");
  await page.locator("html[data-visual-ready]").waitFor();

  const plan = page.getByRole("button", { name: "Step 2 / 3" });
  const goal = page.locator('[data-slot="goal-status-row"]');
  const planBox = await plan.boundingBox();
  const goalBox = await goal.boundingBox();

  expect(planBox).not.toBeNull();
  expect(goalBox).not.toBeNull();
  expect(goalBox!.y - (planBox!.y + planBox!.height)).toBeGreaterThanOrEqual(0);
  expect(goalBox!.y - (planBox!.y + planBox!.height)).toBeLessThanOrEqual(16);
});

test("the standing goal stays in the composer stack instead of claiming the transcript header", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=running");
  await page.locator("html[data-visual-ready]").waitFor();

  const goal = page.locator('[data-slot="composer-top-tray-surface"]');
  const composer = page.locator('[data-slot="composer-root"]');
  const goalBox = await goal.boundingBox();
  const composerBox = await composer.boundingBox();

  expect(goalBox).not.toBeNull();
  expect(composerBox).not.toBeNull();
  expect(Math.abs(composerBox!.x - goalBox!.x)).toBeLessThanOrEqual(1);
  expect(Math.abs(composerBox!.width - goalBox!.width)).toBeLessThanOrEqual(1);
  expect(composerBox!.y - (goalBox!.y + goalBox!.height)).toBeGreaterThanOrEqual(-1);
  expect(composerBox!.y - (goalBox!.y + goalBox!.height)).toBeLessThanOrEqual(0);
  await expect(goal.locator('[data-slot="goal-glyph"]')).toBeVisible();
});

test("the composer context ring exposes the Runtime window occupancy", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=running");
  await page.locator("html[data-visual-ready]").waitFor();

  const readout = contextUsageReadout(VISUAL_CONTEXT_TOKENS, VISUAL_PRIMARY_MODEL_CONTEXT_WINDOW);
  expect(readout).not.toBeNull();
  if (!readout) throw new Error("the canonical context fixture must produce a readout");

  const gauge = page.getByRole("img", { name: `Context usage: ${readout.percent}%` });
  await expect(gauge).toBeVisible();
  await expectStableBox(gauge);
  await gauge.hover();

  const tooltip = page.getByRole("tooltip");
  await expect(tooltip).toContainText("Context window:");
  await expect(tooltip).toContainText(`${readout.percent}% used (${100 - readout.percent}% left)`);
  await expect(tooltip).toContainText(
    `${fmtTokens(readout.usedTokens)} / ${fmtTokens(readout.windowTokens)} tokens used`,
  );
});

for (const theme of ["light", "dark"] as const) {
  test(`a ${theme} user turn uses the Codex-neutral bubble material and geometry`, async ({
    page,
  }) => {
    await page.goto(`/visual/?fixture=agent&theme=${theme}&state=idle`);
    await page.locator("html[data-visual-ready]").waitFor();

    const bubble = page.locator("[data-user-message-bubble]");
    await expect(bubble).toHaveCount(1);
    await expect(bubble).toContainText("Review the Runtime boundary");

    const material = await bubble.evaluate((element) => {
      const probe = document.createElement("div");
      probe.style.background = "color-mix(in srgb, var(--color-text) 5%, transparent)";
      document.body.append(probe);
      const expectedBackground = getComputedStyle(probe).backgroundColor;
      probe.remove();

      const actual = getComputedStyle(element);
      return {
        background: actual.backgroundColor,
        expectedBackground,
        maxWidth: actual.maxWidth,
        padding: [actual.paddingTop, actual.paddingRight, actual.paddingBottom, actual.paddingLeft],
        radius: actual.borderRadius,
        superellipse: CSS.supports("corner-shape", "superellipse(1.5)"),
      };
    });

    expect(material).toEqual({
      background: material.expectedBackground,
      expectedBackground: material.expectedBackground,
      maxWidth: "70%",
      padding: ["8px", "12px", "8px", "12px"],
      radius: material.superellipse ? "20px" : "16px",
      superellipse: material.superellipse,
    });
  });
}

test("composer keeps one production edge and 6/8 footer inset", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=empty");
  await page.locator("html[data-visual-ready]").waitFor();

  const composer = page.locator('[data-slot="composer-root"]');
  const footer = page.locator('[data-slot="composer-footer"]');
  await expect(footer).toHaveCSS("padding-bottom", "6px");
  await expect(footer).toHaveCSS("padding-right", "8px");
  await expect(page.getByRole("button", { name: "Attach image" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Switch model" })).toBeVisible();

  const box = await composer.boundingBox();
  expect(box?.width).toBe(
    await page.evaluate(() =>
      Number.parseFloat(
        getComputedStyle(document.documentElement).getPropertyValue("--content-max"),
      ),
    ),
  );

  await expect(composer).toHaveCSS("border-top-width", "0px");
  const material = await composer.evaluate((element) => {
    const probe = document.createElement("div");
    probe.style.boxShadow =
      "0 0 0 var(--composer-edge-width) color-mix(in oklab, var(--color-text) 14%, transparent), var(--shadow-composer-depth)";
    probe.style.background = "var(--app-composer-surface)";
    probe.style.backdropFilter = "var(--composer-backdrop)";
    document.body.append(probe);
    const expected = {
      shadow: getComputedStyle(probe).boxShadow,
      fill: getComputedStyle(probe).backgroundColor,
      backdrop: getComputedStyle(probe).backdropFilter,
    };
    probe.remove();
    const actual = getComputedStyle(element);
    return {
      expected,
      shadow: actual.boxShadow,
      fill: actual.backgroundColor,
      backdrop: actual.backdropFilter,
    };
  });
  expect(material.shadow).toBe(material.expected.shadow);
  expect(material.fill).toBe(material.expected.fill);
  expect(material.fill).toMatch(/rgba|color\(|\/\s*0?\.\d/);
  expect(material.backdrop).toBe(material.expected.backdrop);
  expect(material.backdrop).not.toBe("none");

  const ringBeforeFocus = material.shadow;
  await page.getByRole("textbox", { name: "Message composer" }).focus();
  await expect
    .poll(() => composer.evaluate((element) => getComputedStyle(element).boxShadow))
    .not.toBe(ringBeforeFocus);
});

test("model capabilities drive the picker and image admission together", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=empty");
  await page.locator("html[data-visual-ready]").waitFor();

  const attach = page.getByRole("button", { name: "Attach image" });
  const effort = page.getByRole("button", { name: "Switch reasoning effort" });
  await expect(attach).toBeEnabled();
  await expect(effort).toHaveText("medium");
  await effort.click();
  await page.getByRole("menuitem", { name: "high", exact: true }).click();
  await expect(effort).toHaveText("high");
  await page.getByRole("button", { name: "Switch model" }).click();
  await expect(page.getByRole("option", { name: GPT_5_6_SOL_CAPABILITY_NAME })).toBeVisible();
  await page.getByPlaceholder("Search models…").fill("Qwen MT Plus");
  await page.getByRole("option", { name: QWEN_MT_PLUS_CAPABILITY_NAME }).click();

  await expect(attach).toBeDisabled();
  await expect(effort).toHaveCount(0);
});

for (const theme of ["light", "dark"] as const) {
  test(`the model picker opens on one provider over a fixed measure in ${theme}`, async ({
    page,
  }) => {
    await page.goto(`/visual/?fixture=agent&theme=${theme}&state=empty`);
    await page.locator("html[data-visual-ready]").waitFor();

    await page.getByRole("button", { name: "Switch model" }).click();
    const surface = page.getByRole("dialog", { name: "Switch model" });
    await expect(surface).toBeVisible();
    await expect(page.getByPlaceholder("Search models…")).toBeFocused();

    const rail = surface.locator("button[aria-pressed]");
    await expect(rail.first()).toHaveAttribute("aria-pressed", "true");

    const body = surface.locator("button[aria-pressed]").first().locator("..").locator("..");
    await expectStableBox(body);
    const before = await body.boundingBox();

    await rail.last().click();
    await expect(rail.last()).toBeFocused();
    await expectStableBox(body);
    expect((await body.boundingBox())!.height).toBe(before!.height);

    await page.getByPlaceholder("Search models…").fill("gpt");
    await expect(surface.locator("button[aria-pressed]")).toHaveCount(0);

    await page.keyboard.press("Escape");
    await expect(surface).toBeVisible();
    await expect(page.getByPlaceholder("Search models…")).toHaveValue("");
    await expect(rail.first()).toBeVisible();

    await rail.first().click();

    const row = page.getByRole("option").first();
    await expect(row).not.toHaveAttribute("data-highlighted", "");
    await row.hover();
    await expect(row).toHaveAttribute("data-highlighted", "");
    await expect(row).not.toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
    await page.mouse.move(0, 0);

    await expectStableBox(surface);
    await expect(surface).toHaveScreenshot(`model-picker-${theme}.png`);
  });
}

test("the projectless composer owns Codex's inset rear project tray", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=empty");
  await page.locator("html[data-visual-ready]").waitFor();

  const tray = page.locator('[data-slot="composer-top-tray-surface"]');
  const composer = page.locator('[data-slot="composer-root"]');
  const footer = page.locator('[data-slot="composer-footer"]');
  const trayBox = await tray.boundingBox();
  const composerBox = await composer.boundingBox();

  expect(trayBox).not.toBeNull();
  expect(composerBox).not.toBeNull();
  expect(trayBox!.x - composerBox!.x).toBe(12);
  expect(composerBox!.x + composerBox!.width - (trayBox!.x + trayBox!.width)).toBe(12);
  expect(composerBox!.y - trayBox!.y).toBe(37);
  expect(trayBox!.y + trayBox!.height - composerBox!.y).toBe(22);
  await expect(tray.getByRole("button", { name: "Choose project" })).toBeVisible();
  await expect(tray.locator("svg")).toHaveCount(1);
  await expect(footer.getByRole("button", { name: "Choose project" })).toHaveCount(0);
});

test("recovery action dismisses the problem and resends the last user input", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=recovery");
  await page.locator("html[data-visual-ready]").waitFor();

  await page.getByTestId("agent-state").getByRole("button", { name: "Retry" }).click();

  await expect(page.getByRole("alert")).toHaveCount(0);
  await expect(page.locator("html")).toHaveAttribute(
    "data-visual-sent-input",
    /Review the Runtime boundary/,
  );
});

test("long content remains inside the reading column without horizontal overflow", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=long-content");
  await page.locator("html[data-visual-ready]").waitFor();

  const stream = page.locator(".msg-scroll-viewport");
  const overflow = await stream.evaluate((element) => element.scrollWidth - element.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
  await expect(page.locator('[data-slot="composer-root"]')).toBeVisible();
});

test("historical turns skip off-screen rendering and the tail turn never does", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=narrative");
  await page.locator("html[data-visual-ready]").waitFor();

  const turns = page.locator("[data-turn-id]");
  const count = await turns.count();
  expect(count).toBeGreaterThan(1);

  const visibility = await turns.evaluateAll((nodes) =>
    nodes.map((node) => getComputedStyle(node).contentVisibility),
  );
  expect(visibility.slice(0, -1)).toEqual(Array(count - 1).fill("auto"));
  expect(visibility.at(-1)).toBe("visible");
});

async function layOutTranscript(page: Page): Promise<void> {
  await page.evaluate(async () => {
    const scroller = document.querySelector(".msg-scroll-viewport");
    if (!scroller) return;
    const frame = () => new Promise((resolve) => requestAnimationFrame(() => resolve(null)));
    for (let top = 0; top <= scroller.scrollHeight; top += scroller.clientHeight) {
      scroller.scrollTop = top;
      await frame();
      await frame();
    }
  });
}

async function expectStableBox(locator: Locator): Promise<void> {
  let previous = "";
  await expect
    .poll(async () => {
      const box = await locator.boundingBox();
      const current = box ? `${box.x},${box.y},${box.width},${box.height}` : "";
      const settled = current !== "" && current === previous;
      previous = current;
      return settled;
    })
    .toBe(true);
}

async function expectRevealOnHover(target: Locator, revealed: Locator): Promise<void> {
  await expectStableBox(target);
  await expect(async () => {
    await target.hover();
    expect(await revealed.evaluate((node) => getComputedStyle(node).opacity)).toBe("1");
  }).toPass();
}

test("code blocks stay readable and expose the wrap control", async ({ context, page }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"], {
    origin: "http://127.0.0.1:4174",
  });
  await page.goto("/visual/?fixture=agent&theme=light&state=long-content");
  await page.locator("html[data-visual-ready]").waitFor();

  const code = page.locator(".shiki-block").filter({ hasText: "Execute(context.Context" });
  await expect(code).toContainText("Execute(context.Context");
  const wrapControls = page.getByRole("button", { name: "Enable word wrap" });
  await expect(wrapControls).toHaveCount(3);
  await expect(wrapControls.first()).toHaveAttribute("aria-pressed", "false");
  await wrapControls.first().click();

  const wrappedControls = page.getByRole("button", { name: "Disable word wrap" });
  await expect(wrappedControls).toHaveCount(3);
  await expect(wrappedControls.first()).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator('.shiki-body[data-wrap="true"]')).toHaveCount(3);
  await expect(page.locator("iframe")).toHaveCount(0);
  await expect(page.locator(".shiki-block").filter({ hasText: "parent.postMessage" })).toHaveCount(
    1,
  );

  await code.evaluate((element) => {
    const range = document.createRange();
    range.selectNodeContents(element);
    const selection = getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
  });
  await page.keyboard.press("ControlOrMeta+C");
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe(
      [
        "type Executor interface {",
        "    Execute(context.Context, Request) (Result, error)",
        "}",
      ].join("\n"),
    );

  const svgPreview = page.getByRole("img", { name: "Image generated by the assistant" });
  await expect(svgPreview).toBeVisible();
  const svgArtifact = page.locator(".shiki-block").filter({ has: svgPreview });
  const svgCopy = svgArtifact.getByRole("button", { name: "Copy code" });
  await expect
    .poll(() => svgPreview.evaluate((image: HTMLImageElement) => image.naturalWidth))
    .toBe(240);
  await page.mouse.move(0, 0);
  await expect.poll(() => svgCopy.evaluate((button) => getComputedStyle(button).opacity)).toBe("0");
  await expectRevealOnHover(svgArtifact, svgCopy);
  await expect(svgArtifact.locator('[data-slot="shiki-preview-body"]')).toHaveAttribute(
    "tabindex",
    "0",
  );
});

test("code blocks use the Codex caption and source geometry", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=long-content");
  await page.locator("html[data-visual-ready]").waitFor();

  const block = page.locator(".shiki-block").filter({ hasText: "Execute(context.Context" });
  await block.locator(".shiki").waitFor();
  await block.locator('[data-markdown-copy="exclude"]').waitFor();
  const geometry = await block.evaluate((root) => {
    const header = root.querySelector<HTMLElement>('[data-markdown-copy="exclude"]');
    const language = Array.from(header?.querySelectorAll("span") ?? []).find(
      (element) => element.textContent?.trim() === "go",
    );
    const source = root.querySelector<HTMLElement>(".shiki");
    if (!header || !language || !source) return null;
    const headerStyle = getComputedStyle(header);
    const languageStyle = getComputedStyle(language);
    const sourceStyle = getComputedStyle(source);
    const blockStyle = getComputedStyle(root);
    return {
      headerBackground: headerStyle.backgroundColor,
      blockMargin: blockStyle.marginBlockStart,
      headerPadding: `${headerStyle.paddingBlockStart} ${headerStyle.paddingInlineStart}`,
      bodyFamily: getComputedStyle(document.body).fontFamily,
      languageFamily: languageStyle.fontFamily,
      languageSize: languageStyle.fontSize,
      languageTransform: languageStyle.textTransform,
      sourcePadding: sourceStyle.paddingInlineStart,
      sourceMaxHeight: sourceStyle.maxHeight,
    };
  });

  expect(geometry).not.toBeNull();
  expect.soft(geometry?.headerBackground).toBe("rgba(0, 0, 0, 0)");
  expect.soft(geometry?.blockMargin).toBe("14px");
  expect.soft(geometry?.headerPadding).toBe("4px 8px");
  expect.soft(geometry?.languageFamily).toBe(geometry?.bodyFamily);
  expect.soft(geometry?.languageSize).toBe("14px");
  expect.soft(geometry?.languageTransform).toBe("none");
  expect.soft(geometry?.sourcePadding).toBe("8px");
  expect.soft(geometry?.sourceMaxHeight).toBe("none");
});

for (const theme of ["light", "dark"] as const) {
  test(`code block keeps its Codex material ${theme}`, async ({ page }) => {
    await page.goto(`/visual/?fixture=agent&theme=${theme}&state=long-content`);
    await page.locator("html[data-visual-ready]").waitFor();
    const block = page.locator(".shiki-block").filter({ hasText: "Execute(context.Context" });
    await expect(block).toHaveScreenshot(`markdown-code-block-${theme}.png`);
  });
}

for (const theme of ["light", "dark"] as const) {
  test(`Mermaid is a semantic, copyable, zoomable artifact ${theme}`, async ({ context, page }) => {
    await context.grantPermissions(["clipboard-read", "clipboard-write"], {
      origin: "http://127.0.0.1:4174",
    });
    await page.goto(`/visual/?fixture=agent&theme=${theme}&state=long-content`);
    await page.locator("html[data-visual-ready]").waitFor();

    const diagram = page.getByRole("img", { name: "Diagram" });
    await expect(diagram).toBeVisible();
    const artifact = diagram.locator("..");
    await expectStableBox(artifact);
    await artifact.hover();
    await expect(artifact).toHaveScreenshot(`markdown-mermaid-${theme}.png`, {
      maxDiffPixels: 400,
    });

    await artifact.getByRole("button", { name: "Copy Mermaid" }).click();
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toContain("```mermaid\ngraph LR");

    await artifact.evaluate((element) => {
      const range = document.createRange();
      range.selectNodeContents(element);
      const selection = getSelection();
      selection?.removeAllRanges();
      selection?.addRange(range);
    });
    await page.keyboard.press("ControlOrMeta+C");
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe("```mermaid\ngraph LR\n  Runtime --> Desktop\n  Desktop --> Frontend\n```");

    await artifact.getByRole("button", { name: "Enlarge diagram" }).click();
    const enlarged = page.getByRole("dialog", { name: "Diagram" });
    await expect(enlarged).toBeVisible();
    await expect(enlarged).toHaveScreenshot(`markdown-mermaid-full-${theme}.png`, {
      maxDiffPixels: 700,
    });
    await page.keyboard.press("Escape");
  });

  test(`Markdown tables open a Codex reading preview ${theme}`, async ({ page }) => {
    await page.goto(`/visual/?fixture=agent&theme=${theme}&state=long-content`);
    await page.locator("html[data-visual-ready]").waitFor();

    const table = page.locator("[data-markdown-table]").filter({ hasText: "Boundary" });
    await layOutTranscript(page);
    await table.evaluate((element) => element.scrollIntoView({ block: "center" }));
    await expectStableBox(table);
    await table.hover();
    await page.getByRole("button", { name: "Expand table" }).click();

    const dialog = page.getByRole("dialog", { name: "Table preview" });
    await expect(dialog).toBeVisible();
    await page.evaluate(() => document.fonts.ready);
    await expect(dialog).toHaveCSS("scale", "none");
    await page.evaluate(
      () =>
        new Promise<void>((resolve) =>
          requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
        ),
    );
    await expect(dialog).toHaveScreenshot(`markdown-table-preview-${theme}.png`);
    await page.getByRole("button", { name: "Close table preview" }).click();
    await expect(dialog).toHaveCount(0);
  });
}

for (const theme of ["light", "dark"] as const) {
  test(`tables keep semantic alignment and copy their Markdown source ${theme}`, async ({
    context,
    page,
  }) => {
    await context.grantPermissions(["clipboard-read", "clipboard-write"], {
      origin: "http://127.0.0.1:4174",
    });
    await page.goto(`/visual/?fixture=agent&theme=${theme}&state=long-content`);
    await page.locator("html[data-visual-ready]").waitFor();

    const table = page.locator("[data-markdown-table]").filter({ hasText: "Run lifecycle" });
    await expect(table.locator("table")).toHaveAttribute("dir", "auto");
    await expect(table.locator("td.md-table-cell-numeric")).toHaveCount(2);

    await expectStableBox(table);
    await table.hover();
    await expect(table).toHaveScreenshot(`markdown-table-${theme}.png`);
    await table.getByRole("button", { name: "Copy table" }).click();
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toContain("| Boundary | Owner | Checks |");
  });
}

for (const theme of ["light", "dark"] as const) {
  test(`Markdown media previews inline data without requesting remote URLs ${theme}`, async ({
    page,
  }) => {
    let remoteRequests = 0;
    await page.route("https://tracker.example/**", async (route) => {
      remoteRequests += 1;
      await route.abort();
    });
    await page.goto(`/visual/?fixture=agent&theme=${theme}&state=long-content`);
    await page.locator("html[data-visual-ready]").waitFor();

    const blocked = page.getByRole("button", { name: "Tracking pixel" });
    await expect(blocked).toBeDisabled();
    await expect(page.locator('img[src^="https://tracker.example/"]')).toHaveCount(0);
    expect(remoteRequests).toBe(0);

    const preview = page.getByRole("button", { name: "Inline architecture" });
    await expect(page.locator('[data-markdown-image-grid="true"] > button')).toHaveCount(2);
    const unseen = await preview.locator("img").boundingBox();
    expect(unseen?.height ?? 0, "an image below the fold holds no box").toBeGreaterThan(8);
    await layOutTranscript(page);
    await preview.evaluate((button) => button.parentElement?.scrollIntoView({ block: "center" }));
    await expect(preview).toBeVisible();
    await expectStableBox(preview);
    await expect(preview).toHaveScreenshot(`markdown-image-${theme}.png`);
    await preview.click();
    const dialog = page.getByRole("dialog", { name: "Inline architecture" });
    await expect(dialog).toBeVisible();
    const transcript = page.locator(".msg-scroll-viewport");
    await transcript.evaluate((viewport) => {
      viewport.scrollTop = 0;
    });
    await expect.poll(() => transcript.evaluate((viewport) => viewport.scrollTop)).toBe(0);
    const controlSizes = await Promise.all(
      ["Download image", "Close image preview", "Zoom out image", "Zoom in image"].map((name) =>
        page.getByRole("button", { name }).evaluate((button, accessibleName) => {
          const element = button as HTMLElement;
          return {
            accessibleName,
            width: element.offsetWidth,
            height: element.offsetHeight,
          };
        }, name),
      ),
    );
    for (const { accessibleName, width, height } of controlSizes) {
      expect.soft(width, `${accessibleName} width`).toBeGreaterThanOrEqual(40);
      expect.soft(height, `${accessibleName} height`).toBeGreaterThanOrEqual(40);
    }
    await expect(dialog).toHaveScreenshot(`markdown-image-lightbox-${theme}.png`);
    await page.getByRole("button", { name: "Zoom in image" }).click();
    await expect(dialog.locator('[data-image-zoom="125"]')).toBeVisible();
    await page.getByRole("button", { name: "Next image" }).click();
    await expect(page.getByRole("dialog", { name: "Inline detail" })).toBeVisible();
    await expect(page.locator('[data-image-zoom="100"]')).toBeVisible();
    await page.keyboard.press("ArrowLeft");
    await expect(page.getByRole("dialog", { name: "Inline architecture" })).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.getByRole("dialog", { name: "Inline architecture" })).toHaveCount(0);
  });
}

test("context compaction uses the Codex activity row without divider chrome", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=narrative");
  await page.locator("html[data-visual-ready]").waitFor();

  const compaction = page.getByRole("button", { name: "Context automatically compacted" });
  await compaction.scrollIntoViewIfNeeded();
  await expect(compaction.locator('[data-icon-name="minimize"]')).toBeVisible();
  const row = compaction.locator("xpath=..");
  const chrome = await row.evaluate((node) => {
    const kids = [...node.querySelectorAll("*")];
    const hairlines = kids.filter((element) => {
      const box = element.getBoundingClientRect();
      const style = getComputedStyle(element);
      const painted =
        style.backgroundColor !== "rgba(0, 0, 0, 0)" ||
        Number.parseFloat(style.borderTopWidth) > 0 ||
        Number.parseFloat(style.borderBottomWidth) > 0;
      return box.height > 0 && box.height <= 1.5 && box.width >= 24 && painted;
    });
    return { examined: kids.length, hairlines: hairlines.map((element) => element.tagName) };
  });
  expect(chrome.examined, "the compaction row has to have something in it").toBeGreaterThan(3);
  expect(chrome.hairlines, "divider chrome beside the compaction row").toEqual([]);
  await expect(compaction).toHaveAttribute("aria-expanded", "false");
  await expect(compaction.locator("xpath=..")).toHaveScreenshot("context-compaction-light.png");

  await compaction.click();
  await expect(compaction).toHaveAttribute("aria-expanded", "true");
  await expect(page.getByText("Earlier tool output folded into a summary.")).toBeVisible();
});

test("Markdown structural primitives follow the Codex reading grammar", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=long-content");
  await page.locator("html[data-visual-ready]").waitFor();

  const markdown = page.locator(".md").filter({ hasText: "Structural primitives" });
  const styles = await markdown.evaluate((root) => {
    const level2 = Array.from(root.querySelectorAll('[data-md-level="2"]')).find((heading) =>
      heading.textContent?.includes("Architecture review"),
    );
    const level3 = Array.from(root.querySelectorAll('[data-md-level="3"]')).find((heading) =>
      heading.textContent?.includes("Structural primitives"),
    );
    const primaryList = Array.from(root.querySelectorAll("ul")).find((list) =>
      list.textContent?.includes("Primary marker"),
    );
    const leadParagraph = Array.from(root.querySelectorAll(":scope > p")).find((paragraph) =>
      paragraph.textContent?.includes("The consumer owns persistence policy"),
    );
    const leadList = leadParagraph?.nextElementSibling;
    const tableContainer = root.querySelector(".md-table-container");
    const table = tableContainer?.querySelector("table");
    const tableHeader = table?.querySelector("th");
    const proseParagraph = Array.from(root.querySelectorAll(":scope > p")).find((paragraph) =>
      paragraph.textContent?.includes("A deliberately long final paragraph"),
    );
    const inlineCode = proseParagraph?.querySelector("code");
    const nestedList = primaryList?.querySelector(":scope > li > ul");
    const deepList = nestedList?.querySelector(":scope > li > ul");
    const rtlList = Array.from(root.querySelectorAll("ul")).find((list) =>
      list.textContent?.includes("المرحلة الأولى"),
    );
    const taskList = root.querySelector("ol.contains-task-list");
    const looseTask = taskList?.querySelector("li.task-list-item:has(> p)");
    const looseTaskCheckbox = looseTask?.firstElementChild;
    const looseTaskParagraphs = looseTask?.querySelectorAll(":scope > p");
    const quote = root.querySelector("blockquote");
    const rule = root.querySelector("hr");
    if (
      !level2 ||
      !level3 ||
      !leadParagraph ||
      !(leadList instanceof HTMLUListElement) ||
      !tableContainer ||
      !table ||
      !tableHeader ||
      !proseParagraph ||
      !inlineCode ||
      !nestedList ||
      !deepList ||
      !rtlList ||
      !taskList ||
      !looseTask ||
      !(looseTaskCheckbox instanceof HTMLInputElement) ||
      looseTaskParagraphs?.length !== 2 ||
      !quote ||
      !rule
    )
      return null;
    const level2Style = getComputedStyle(level2);
    const level3Style = getComputedStyle(level3);
    const leadParagraphStyle = getComputedStyle(leadParagraph);
    const leadListStyle = getComputedStyle(leadList);
    const tableContainerStyle = getComputedStyle(tableContainer);
    const tableStyle = getComputedStyle(table);
    const tableHeaderStyle = getComputedStyle(tableHeader);
    const proseParagraphStyle = getComputedStyle(proseParagraph);
    const inlineCodeStyle = getComputedStyle(inlineCode);
    const rtlListStyle = getComputedStyle(rtlList);
    return {
      level2Tag: level2.tagName,
      level2Size: level2Style.fontSize,
      level2Margin: `${level2Style.marginBlockStart} ${level2Style.marginBlockEnd}`,
      level3Tag: level3.tagName,
      level3Size: level3Style.fontSize,
      level3Margin: `${level3Style.marginBlockStart} ${level3Style.marginBlockEnd}`,
      leadParagraphMargin: `${leadParagraphStyle.marginBlockStart} ${leadParagraphStyle.marginBlockEnd}`,
      leadListMargin: `${leadListStyle.marginBlockStart} ${leadListStyle.marginBlockEnd}`,
      tableMargin: `${tableContainerStyle.marginBlockStart} ${tableContainerStyle.marginBlockEnd}`,
      tableFontSize: tableStyle.fontSize,
      tableLineHeight: tableStyle.lineHeight,
      tableHeaderFontSize: tableHeaderStyle.fontSize,
      tableHeaderLineHeight: tableHeaderStyle.lineHeight,
      proseParagraphMargin: `${proseParagraphStyle.marginBlockStart} ${proseParagraphStyle.marginBlockEnd}`,
      inlineCodeDecoration:
        inlineCodeStyle.getPropertyValue("box-decoration-break") ||
        inlineCodeStyle.getPropertyValue("-webkit-box-decoration-break"),
      inlineCodeFontSize: inlineCodeStyle.fontSize,
      inlineCodeRadius: inlineCodeStyle.borderRadius,
      inlineCodeWordBreak: inlineCodeStyle.wordBreak,
      inlineCodeWrap: inlineCodeStyle.overflowWrap,
      rtlDirection: rtlListStyle.direction,
      rtlStartPadding: rtlListStyle.paddingInlineStart,
      rtlEndPadding: rtlListStyle.paddingInlineEnd,
      nestedMarker: getComputedStyle(nestedList).listStyleType,
      deepMarker: getComputedStyle(deepList).listStyleType,
      taskMarker: getComputedStyle(taskList).listStyleType,
      looseTaskDisplay: getComputedStyle(looseTask).display,
      looseTaskColumns: getComputedStyle(looseTask).gridTemplateColumns,
      looseTaskCheckboxInset: getComputedStyle(looseTaskCheckbox).marginTop,
      looseTaskFollowUpColumn: getComputedStyle(looseTaskParagraphs[1]!).gridColumnStart,
      quoteInset: getComputedStyle(quote).paddingInlineStart,
      quoteRule: getComputedStyle(quote, "::after").width,
      ruleMargin: getComputedStyle(rule).marginBlockStart,
    };
  });

  expect(styles).not.toBeNull();
  expect.soft(styles?.level2Tag).toBe("H3");
  expect.soft(styles?.level2Size).toBe("20px");
  expect.soft(styles?.level2Margin).toBe("20px 10px");
  expect.soft(styles?.level3Tag).toBe("H4");
  expect.soft(styles?.level3Size).toBe("17px");
  expect.soft(styles?.level3Margin).toBe("20px 10px");
  expect.soft(styles?.leadParagraphMargin).toBe("0px 10px");
  expect.soft(styles?.leadListMargin).toBe("0px 10px");
  expect.soft(styles?.tableMargin).toBe("0px 0px");
  expect.soft(styles?.tableFontSize).toBe("14px");
  expect.soft(styles?.tableLineHeight).toBe("21px");
  expect.soft(styles?.tableHeaderFontSize).toBe("14px");
  expect.soft(styles?.tableHeaderLineHeight).toBe("16px");
  expect.soft(styles?.proseParagraphMargin).toBe("0px 11px");
  expect.soft(styles?.inlineCodeDecoration).toBe("clone");
  expect.soft(styles?.inlineCodeFontSize).toBe("14.72px");
  expect.soft(styles?.inlineCodeRadius).toBe("6px");
  expect.soft(styles?.inlineCodeWordBreak).toBe("break-word");
  expect.soft(styles?.inlineCodeWrap).toBe("anywhere");
  expect.soft(styles?.rtlDirection).toBe("rtl");
  expect.soft(styles?.rtlStartPadding).toBe("21px");
  expect.soft(styles?.rtlEndPadding).toBe("0px");
  expect.soft(styles?.nestedMarker).toBe("circle");
  expect.soft(styles?.deepMarker).toBe("square");
  expect.soft(styles?.taskMarker).toBe("none");
  expect.soft(styles?.looseTaskDisplay).toBe("grid");
  expect.soft(styles?.looseTaskColumns).not.toBe("none");
  expect.soft(styles?.looseTaskCheckboxInset).toBe("4px");
  expect.soft(styles?.looseTaskFollowUpColumn).toBe("2");
  expect.soft(styles?.quoteInset).toBe("24px");
  expect.soft(styles?.quoteRule).toBe("4px");
  expect.soft(styles?.ruleMargin).toBe("28px");
});

for (const theme of ["light", "dark"] as const) {
  test(`wrapped inline code keeps the Codex cloned well in ${theme}`, async ({ page }) => {
    await page.goto(`/visual/?fixture=agent&theme=${theme}&state=long-content`);
    await page.locator("html[data-visual-ready]").waitFor();

    const paragraph = page
      .locator(".md > p")
      .filter({ hasText: "A deliberately long final paragraph" });
    const inlineCode = paragraph.locator("code");
    await expect(inlineCode).toContainText("expectedRuntimeProjectionRevisionIdentifier");
    expect(await inlineCode.evaluate((element) => element.getClientRects().length)).toBeGreaterThan(
      1,
    );
    await expect(paragraph).toHaveScreenshot(`inline-code-wrap-${theme}.png`);
  });
}

test("every chrome bar that takes a bottom edge wears the style edge", async ({ page }) => {
  await page.goto("/visual/?fixture=workspace&theme=light&state=dock-light");
  await page.locator("html[data-visual-ready]").waitFor();

  const measured = await page.evaluate(() => {
    const probe = document.createElement("div");
    probe.style.boxShadow = "var(--app-header-edge) var(--color-border)";
    document.body.append(probe);
    const edge = getComputedStyle(probe).boxShadow;
    probe.remove();
    const bars = [...document.querySelectorAll(".agent-surface-header")];
    return {
      edge,
      withEdge: bars
        .filter((bar) => bar.classList.contains("agent-surface-divider"))
        .map((bar) => getComputedStyle(bar).boxShadow),
      withoutEdge: bars
        .filter((bar) => !bar.classList.contains("agent-surface-divider"))
        .map((bar) => getComputedStyle(bar).boxShadow),
    };
  });

  expect(measured.withEdge.length).toBeGreaterThanOrEqual(2);
  for (const shadow of measured.withEdge) expect(shadow).toBe(measured.edge);
  for (const shadow of measured.withoutEdge) expect(shadow).toBe("none");
});

for (const { state, inputSurface } of [
  { state: "long-content", inputSurface: '[data-slot="composer-root"]' },
  { state: "question", inputSurface: '[data-slot="question-request-surface"]' },
  { state: "delegated", inputSurface: '[data-slot="composer-root"]' },
] as const) {
  test(`the floating input surface reserves its own height at the tail of ${state}`, async ({
    page,
  }) => {
    await page.goto(`/visual/?fixture=agent&theme=light&state=${state}`);
    await page.locator("html[data-visual-ready]").waitFor();

    const measured = await page.evaluate(async (inputSurface) => {
      const scroller = document.querySelector(".msg-scroll-viewport");
      const input = document.querySelector(inputSurface);
      if (!scroller || !input) return null;
      scroller.scrollTop = scroller.scrollHeight;
      await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
      const tail = scroller.firstElementChild?.lastElementChild;
      if (!tail) return null;
      return {
        clearance: Math.round(
          input.getBoundingClientRect().top - tail.getBoundingClientRect().bottom,
        ),
        margin: Math.round(Number.parseFloat(getComputedStyle(document.documentElement).fontSize)),
      };
    }, inputSurface);

    expect(measured?.margin).toBeGreaterThan(0);
    expect(measured!.clearance).toBeGreaterThanOrEqual(measured!.margin);
  });
}

for (const { state, action } of [{ state: "waiting", action: "Allow once" }] as const) {
  test(`compact ${state} opens with its blocking action above the composer`, async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 577 });
    await page.goto(`/visual/?fixture=agent&theme=light&state=${state}`);
    await page.locator("html[data-visual-ready]").waitFor();

    const composer = page.locator('[data-slot="composer-root"]');
    const button = page.getByRole("button", { name: action, exact: true });
    await expect(composer).toBeVisible();
    await expect(button).toBeVisible();
    await expect
      .poll(() =>
        page
          .locator(".msg-scroll-viewport")
          .evaluate((element) =>
            Number.parseFloat(getComputedStyle(element).getPropertyValue("--composer-overlay")),
          ),
      )
      .toBeGreaterThan(0);

    const clearance = await Promise.all([button.boundingBox(), composer.boundingBox()]);
    expect(clearance[0]!.y + clearance[0]!.height).toBeLessThanOrEqual(clearance[1]!.y);
  });
}

test("compact question replaces the composer with its blocking request", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 577 });
  await page.goto("/visual/?fixture=agent&theme=light&state=question");
  await page.locator("html[data-visual-ready]").waitFor();

  const request = page.locator('[data-slot="question-request-surface"]');
  const skip = page.getByRole("button", { name: "Skip", exact: true });
  await expect(request).toBeVisible();
  await expect(page.locator('[data-slot="composer-root"]')).toHaveCount(0);
  await expect(skip).toBeVisible();
  await expect
    .poll(() =>
      page
        .locator(".msg-scroll-viewport")
        .evaluate((element) =>
          Number.parseFloat(getComputedStyle(element).getPropertyValue("--composer-overlay")),
        ),
    )
    .toBeGreaterThan(0);

  const [requestBox, skipBox] = await Promise.all([request.boundingBox(), skip.boundingBox()]);
  expect(skipBox!.y).toBeGreaterThanOrEqual(requestBox!.y);
  expect(skipBox!.y + skipBox!.height).toBeLessThanOrEqual(requestBox!.y + requestBox!.height);
});

test("async transcript materialization follows only while the reader stays at the tail", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=long-content");
  await page.locator("html[data-visual-ready]").waitFor();
  await expect(page.locator(".shiki-block .shiki")).toHaveCount(3);
  await expect(page.getByRole("img", { name: "Diagram" })).toBeVisible();

  const measured = await page.evaluate(async () => {
    const scroller = document.querySelector<HTMLElement>(".msg-scroll-viewport");
    const content = scroller?.firstElementChild;
    if (!scroller || !content) return null;

    const grow = (height: number) => {
      const probe = document.createElement("div");
      probe.style.height = `${height}px`;
      probe.style.flex = `0 0 ${height}px`;
      content.append(probe);
      return probe;
    };
    const settle = () =>
      new Promise<void>((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
      );

    scroller.scrollTop = scroller.scrollHeight;
    await settle();
    const followedProbe = grow(160);
    await settle();
    const followedDistance = scroller.scrollHeight - scroller.clientHeight - scroller.scrollTop;

    scroller.dispatchEvent(new WheelEvent("wheel", { bubbles: true, deltaY: -220 }));
    scroller.scrollTop = Math.max(0, scroller.scrollTop - 220);
    await settle();
    const readerTop = scroller.scrollTop;
    const escapedDistance = scroller.scrollHeight - scroller.clientHeight - readerTop;

    const escapedProbe = grow(180);
    await settle();
    const afterGrowth = {
      top: scroller.scrollTop,
      distance: scroller.scrollHeight - scroller.clientHeight - scroller.scrollTop,
    };

    followedProbe.remove();
    escapedProbe.remove();
    return { followedDistance, readerTop, escapedDistance, afterGrowth };
  });

  expect(measured).not.toBeNull();
  expect(measured!.followedDistance).toBeLessThanOrEqual(1);
  expect(measured!.afterGrowth.top).toBe(measured!.readerTop);
  expect(measured!.afterGrowth.distance - measured!.escapedDistance).toBe(180);
});

test("a tool with a standing surface is not narrated as well", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=running");
  await page.locator("html[data-visual-ready]").waitFor();

  const plan = page.getByRole("button", { name: "Step 2 / 3" });
  await expectStableBox(plan);
  await plan.hover();
  await expect(page.getByText("Review visual evidence", { exact: true })).toBeVisible();

  const stream = page.locator(".msg-scroll-viewport");
  for (let i = 0; i < 6; i++) {
    const shut = stream.locator(
      "[data-slot='agent-activity-disclosure'] button[aria-expanded='false']",
    );
    if ((await shut.count()) === 0) break;
    await shut
      .first()
      .click({ timeout: 2000 })
      .catch(() => {});
  }
  await expect(stream.getByText("Update the plan")).toHaveCount(0);

  await expect(stream.getByText("atomicity_and_idempotency.go").first()).toBeVisible();
});

test("an opened but empty answer folds nothing behind it", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=answer-opening");
  await page.locator("html[data-visual-ready]").waitFor();

  const thinking = page
    .locator("[data-slot='agent-activity-disclosure']")
    .filter({ hasText: "Thinking" });
  await expect(thinking.locator("button[aria-expanded]").first()).toHaveAttribute(
    "aria-expanded",
    "true",
  );
  await expect(thinking).toContainText("The framework must expose execution capability");

  await expect(page.getByRole("button", { name: /steps/ })).toHaveCount(0);
});

test("expanded reasoning keeps a quiet identity mark and an aside rule", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=waves");
  await page.locator("html[data-visual-ready]").waitFor();

  const reasoning = page
    .locator("[data-slot='agent-activity-disclosure']")
    .filter({ hasText: "Thinking" });
  const trigger = reasoning.locator("button[aria-expanded]").first();
  const mark = trigger.locator("span[aria-hidden]").first();

  await expect(reasoning).toHaveAttribute("data-shell", "line");
  await expect(mark.locator("svg")).toBeVisible();
  await expect(mark).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await expect(reasoning.getByRole("region")).toHaveCSS("border-left-width", "1px");
});

test("an expanded patch reports only its call-scoped file receipt", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=tool-shells");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.getByRole("button", { name: /steps/ }).first().click();

  const row = page
    .locator(".msg-scroll-viewport button")
    .filter({ hasText: "specialisedPreviewProjections.ts" })
    .first();
  await expect(row).toBeVisible();

  const clipping = await row.evaluate((element) => {
    const activity = element.closest<HTMLElement>("[data-slot='agent-activity-disclosure']");
    if (activity) activity.style.width = "480px";
    const directory = element.querySelector("[dir=rtl]");
    const filename = directory?.nextElementSibling?.nextElementSibling;
    return {
      directoryClipped: !!directory && directory.scrollWidth > directory.clientWidth + 1,
      directoryLost: directory ? directory.scrollWidth - directory.clientWidth : 0,
      filenameLost: filename ? filename.scrollWidth - filename.clientWidth : 0,
      filenameText: filename?.textContent,
    };
  });
  expect(clipping.directoryClipped).toBe(true);
  expect(clipping.directoryLost).toBeGreaterThan(clipping.filenameLost);
  expect(clipping.filenameText).toBe("specialisedPreviewProjections.ts");
  await expect(row).not.toContainText("+");
  await expect(row).not.toContainText("−");

  await row.click();
  const receipt = page.locator('[data-patch-change="modified"]').filter({
    has: page.getByTitle(
      "/Users/visual/scope/desktop/frontend/src/plugins/builtin/chat/tools/application/specialisedPreviewProjections.ts",
    ),
  });
  await expect(receipt).toContainText("Edited");
});

test("tool invocations stay on the transparent Codex work-narrative plane", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=tool-shells");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.getByRole("button", { name: /steps/ }).first().click();

  const rows = page.locator(
    "[data-slot='agent-activity-disclosure'][data-tool='shell'], " +
      "[data-slot='agent-activity-disclosure'][data-tool='apply_patch']",
  );
  expect(await rows.count()).toBeGreaterThanOrEqual(5);

  for (let index = 0; index < (await rows.count()); index += 1) {
    const row = rows.nth(index);
    await expect(row).toHaveAttribute("data-shell", "line");
    await expect(row).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
    await expect(row).toHaveCSS("border-top-width", "0px");
  }

  await page.mouse.move(0, 0);
  const closedChevron = rows
    .filter({ has: page.locator("button[aria-expanded='false']") })
    .first()
    .locator('[data-slot="agent-activity-chevron"]');
  await expect(closedChevron).toHaveCSS("opacity", "0");
});

test("completed work folds before the separate final answer owns message actions", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=tool-shells");
  await page.locator("html[data-visual-ready]").waitFor();

  const assistantTurns = page.getByRole("heading", { name: "Assistant" });
  await expect(assistantTurns).toHaveCount(2);

  const work = assistantTurns.nth(0).locator("..");
  await expect(work.getByRole("button", { name: /6 steps/ })).toBeVisible();
  await expect(work.getByRole("button", { name: "Copy message" })).toHaveCount(0);

  const answer = assistantTurns.nth(1).locator("..");
  await expect(answer).toContainText("The boundary is clean");
  await expect(answer.getByRole("button", { name: "Copy message" })).toBeVisible();
  await expect(answer.getByRole("button", { name: "Regenerate response" })).toBeVisible();
});

test("a multi-file patch receipt puts every path on one left edge", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=tool-shells");
  await page.locator("html[data-visual-ready]").waitFor();

  await page.getByRole("button", { name: /6 steps/ }).click();
  await page.getByText("3 files").first().click();

  const rows = page.locator("[data-patch-change]");
  await expect(rows).toHaveCount(3);
  expect(new Set(await rows.evaluateAll((r) => r.map((e) => e.dataset.patchChange))).size).toBe(3);

  const edges = await rows.evaluateAll((r) =>
    r.map((row) => ({
      verb: row.children[0]!.getBoundingClientRect().x,
      path: row.children[1]!.getBoundingClientRect().x,
    })),
  );
  for (const edge of edges) {
    expect(edge.verb).toBeCloseTo(edges[0]!.verb, 1);
    expect(edge.path).toBeCloseTo(edges[0]!.path, 1);
  }
});

test("an answered question keeps both halves of the exchange", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=narrative");
  await page.locator("html[data-visual-ready]").waitFor();

  const settled = page
    .locator('[data-slot="agent-activity-disclosure"]')
    .filter({ hasText: "Asked" })
    .first();
  await expect(settled).toContainText("2 questions");
  await expect(settled).not.toContainText("idempotency key");

  await settled.getByRole("button").first().click();
  await expect(settled).toContainText("Where should the idempotency key be minted?");
  await expect(settled).toContainText("At checkout");
  await expect(settled).toContainText("Ship behind the existing payments flag.");
});

test("a question the run was canceled out from under says so in one line", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=canceled");
  await page.locator("html[data-visual-ready]").waitFor();

  await expect(page.getByText("Closed without an answer")).toBeVisible();
  await expect(page.getByText("Should the review cover the CLI too?")).toHaveCount(0);
  await expect(page.getByRole("textbox", { name: "Scope" })).toHaveCount(0);
});

test("a second delegation keeps every sub-agent, and its own status column", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=delegated");
  await page.locator("html[data-visual-ready]").waitFor();

  await expect(page.getByRole("button", { name: /\d+ calls/ })).toHaveCount(0);
  await expect(page.getByRole("button", { name: /Sub-agent/ })).toHaveCount(6);
  await expect(page.locator('[data-slot="approval-surface"]')).toBeVisible();

  const ends = await page
    .locator("#item_fanout [data-slot='agent-activity-disclosure']")
    .evaluateAll((rows) =>
      rows.flatMap((row) => {
        const steps = [...row.querySelectorAll("span")].find((span) =>
          /steps?$/.test((span.textContent ?? "").trim()),
        );
        return steps ? [Math.round(steps.getBoundingClientRect().right)] : [];
      }),
    );
  expect(ends).toHaveLength(4);
  expect(new Set(ends).size).toBe(1);
});

const GOAL_CONTROLS: ReadonlyArray<{ state: string; label: string; actions: string[] }> = [
  { state: "running", label: "Pursuing goal", actions: ["Clear goal", "Pause goal", "Edit goal"] },
  { state: "canceled", label: "Goal stalled", actions: ["Clear goal", "Resume goal", "Edit goal"] },
  { state: "terminal", label: "Cost budget reached", actions: ["Clear goal", "Edit goal"] },
  { state: "steer", label: "Finishing goal", actions: ["Clear goal"] },
];

for (const goal of GOAL_CONTROLS) {
  test(`a goal offers only what its status allows — ${goal.state}`, async ({ page }) => {
    await page.goto(`/visual/?fixture=agent&theme=light&state=${goal.state}`);
    await page.locator("html[data-visual-ready]").waitFor();

    const row = page.locator('[data-slot="goal-status-row"]');
    await expect(row).toContainText(goal.label);
    expect(
      await row
        .locator('[data-slot="goal-actions"] button')
        .evaluateAll((buttons) => buttons.map((button) => button.getAttribute("aria-label"))),
    ).toEqual(goal.actions);
  });
}

test("the transcript publishes one heading outline, from the session down", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=narrative");
  await page.locator("html[data-visual-ready]").waitFor();

  const outline = await page.evaluate(() =>
    [...document.querySelectorAll("h1, h2, h3, h4, h5, h6")].map((heading) => ({
      level: Number(heading.tagName.slice(1)),
      authored: heading.getAttribute("data-md-level"),
      text: (heading.textContent ?? "").trim().slice(0, 24),
    })),
  );

  const roots = outline.filter((heading) => heading.level === 1);
  expect(roots).toHaveLength(1);
  expect(roots[0]?.text).toBe("Agent · narrative");

  expect(
    outline.filter((heading) => heading.level === 2 && !heading.authored).length,
  ).toBeGreaterThan(0);
  for (const heading of outline) {
    if (heading.authored) expect.soft(heading.level).toBeGreaterThanOrEqual(3);
  }

  let previous = 0;
  for (const heading of outline) {
    expect.soft(heading.level).toBeLessThanOrEqual(previous + 1);
    previous = heading.level;
  }
});

test("an expanded wave keeps its summary while its rows scroll past", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=tool-shells");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.getByRole("button", { name: /steps/ }).first().click();

  const header = page
    .locator("[data-slot=agent-activity-disclosure] [data-slot=agent-activity-header][data-sticky]")
    .first();
  await expect(header).toBeVisible();

  const stuck = await header.evaluate((element) => {
    const viewport = element.closest(".msg-scroll-viewport");
    const card = element.parentElement;
    if (!viewport || !card) return null;
    const before = element.getBoundingClientRect().top - card.getBoundingClientRect().top;
    viewport.scrollTop = viewport.scrollHeight;
    return {
      before,
      overflow: getComputedStyle(card).overflow,
      position: getComputedStyle(element).position,
    };
  });
  expect(stuck?.position).toBe("sticky");
  expect(stuck?.overflow).toBe("clip");
});

test("the Goal surface stays quiet and omits Runtime constraints", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=running");
  await page.locator("html[data-visual-ready]").waitFor();

  const row = page.locator('[data-slot="goal-status-row"]');
  await expect(row).toContainText("Pursuing goal");
  await expect(row).toContainText("green on Linux");
  await expect(row.getByRole("button", { name: "Clear goal" })).toBeVisible();
  await expect(row.getByRole("button", { name: "Pause goal" })).toBeVisible();
  await expect(row.getByRole("button", { name: "Edit goal" })).toBeVisible();

  await expect(row).not.toContainText("$4.50/$5.00");
  await expect(row).not.toContainText("7/20");
  await expect(row).not.toContainText("31");
  await expect(row.locator("[role=progressbar]")).toHaveCount(0);
});

for (const theme of ["light", "dark"] as const) {
  for (const state of VISUAL_AGENT_STATES) {
    if (state === "delegated") continue;
    test(`agent golden ${theme} ${state}`, async ({ page }) => {
      await page.goto(`/visual/?fixture=agent&theme=${theme}&state=${state}`);
      await page.locator("html[data-visual-ready]").waitFor();
      if (state === "long-content") {
        await expect(page.locator(".shiki-block .shiki")).toHaveCount(3);
        await expect(page.getByRole("img", { name: "Diagram" })).toBeVisible();
      }
      if (state === "tool-shells") {
        await page.getByRole("button", { name: /steps/ }).first().click();
        await page
          .locator('[data-tool="apply_patch"] button[aria-expanded]')
          .filter({ hasText: "specialisedPreviewProjections.ts" })
          .click();
      }
      await layOutTranscript(page);

      await page.waitForFunction(() => {
        const scroller = document.querySelector(".msg-scroll-viewport");
        if (!scroller) return true;
        scroller.scrollTop = scroller.scrollHeight;
        const probe = window as unknown as { settle?: { top: number; frames: number } };
        const settle = (probe.settle ??= { top: -1, frames: 0 });
        if (scroller.scrollTop === settle.top) settle.frames += 1;
        else {
          settle.top = scroller.scrollTop;
          settle.frames = 0;
        }
        return settle.frames >= 5;
      });

      await freezeVisualClock(page);

      await expect(page).toHaveScreenshot(`agent-${theme}-${state}.png`, {
        mask: state === "empty" ? [page.locator('[data-slot="composer-chip-label"]')] : undefined,
      });
    });
  }
}

for (const theme of ["light", "dark"] as const) {
  test(`agent delegated card ${theme}`, async ({ page }) => {
    await page.goto(`/visual/?fixture=agent&theme=${theme}&state=delegated`);
    await page.locator("html[data-visual-ready]").waitFor();
    await layOutTranscript(page);
    await freezeVisualClock(page);

    const card = page.locator('[data-shell="card"]').first();
    await expect(card).toBeVisible();
    await expect(card).toHaveScreenshot(`agent-${theme}-delegated-card.png`);
  });
}

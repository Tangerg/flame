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
  recovery: "finished",
  delegated: "running",
  "long-content": "finished",
  narrative: "finished",
  "tool-shells": "finished",
  waves: "running",
};

const GPT_5_6_SOL_CAPABILITY_NAME =
  "GPT-5.6 Sol 1.1M context · text + image + pdf input · Reasoning none / low / medium / high / xhigh / max · 922k max input · 128k max output · text output · Tools · Structured output · Knowledge 2026-02-16T00:00:00Z";
// Reached through the search, where the list is no longer scoped to one provider, so each
// row says which one it came from — that caption sits in the accessible name too.
const QWEN_MT_PLUS_CAPABILITY_NAME =
  "Qwen MT Plus Alibaba 32.8k context · text input · text output";

// The Record's own exhaustiveness is not enforced by its type — a partial Record
// still typechecks against an index signature — and an absent expectation reads to
// Playwright as "assert the attribute exists", which passes for every value.
// `narrative` had been missing since it was added. A state without an expectation
// is a state nobody is asserting anything about.
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
  // The `bubble` step, same corner the user's own turn takes — 16px base carrying the
  // superellipse compensation. It read 24px until the ladder claimed it: that was Tailwind's
  // own `rounded-3xl`, the one radius in the tree that was neither a ladder step nor scaled
  // with the rest when the corner curve changed.
  await expect(surface).toHaveCSS("border-radius", "20px");
  await expect(surface.getByText("Terminal", { exact: true })).toBeVisible();
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
  // The `bubble` step, same corner the user's own turn takes — 16px base carrying the
  // superellipse compensation. It read 24px until the ladder claimed it: that was Tailwind's
  // own `rounded-3xl`, the one radius in the tree that was neither a ladder step nor scaled
  // with the rest when the corner curve changed.
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
  await expect(rows).toHaveCount(2);

  // Each carries its own child Run's state, and the nested one renders inside the subtree of
  // the item that spawned the first — the tree this state is named for.
  await expect(rows.nth(0)).toContainText("Needs input");
  await expect(rows.nth(1)).toContainText("Running");
  const nesting = await rows.nth(1).evaluate((deep, shallowId) => {
    const shallow = document.getElementById(shallowId);
    return shallow ? shallow.contains(deep) : null;
  }, "item_delegate");
  expect(nesting).toBe(true);

  // A line, not a surface: no fill and no radius of its own.
  const shell = await rows.nth(0).evaluate((row) => {
    const style = getComputedStyle(row);
    return { background: style.backgroundColor, radius: style.borderTopLeftRadius };
  });
  expect(shell.background).toBe("rgba(0, 0, 0, 0)");
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

  await page.getByRole("button", { name: "Step 2 / 3" }).hover();

  // The tooltip's steps come from the session's plan snapshot, not from a
  // per-run plan Item — same three steps, read from where the protocol keeps them.
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

    // Codex gives the human turn a stable semantic hook and a neutral ink wash:
    // the bubble distinguishes ownership without turning every prompt into an
    // accent/status callout. Pin both schemes because a light-only assertion can
    // accidentally accept a translucent accent whose dark result is much louder.
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
      // 70% is the reference's own `--user-chat-width` for a standard bubble; its 456px cap
      // belongs to a compact variant this app has no counterpart for.
      maxWidth: "70%",
      padding: ["8px", "12px", "8px", "12px"],
      // The bubble sits on the `bubble` step — 16px, same base Codex gives it. Where the
      // superellipse corner is drawn, that step carries the 1.25 compensation from
      // globals.css, which is also what Codex renders on a browser that supports it.
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

  // Read from the token rather than restated: the reading column's width is a
  // design decision that moves, and a literal here asserts one revision of it
  // against every later one. What must hold is that the composer spans the column.
  const box = await composer.boundingBox();
  expect(box?.width).toBe(
    await page.evaluate(() =>
      Number.parseFloat(
        getComputedStyle(document.documentElement).getPropertyValue("--content-max"),
      ),
    ),
  );

  // ONE edge mechanism, and for a panel resting ON the transcript that is a ring,
  // not a border: a drawn line was the only outlined object left on a screen whose
  // regions all separate by cast. So no border AND no second stroke — the ring and
  // the depth under it are the single `box-shadow` this asserts.
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
  // Translucent and blurred, or the ring reads as a stroke around a box rather than
  // as the edge of glass — the material is half of why the border could go.
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
  // The model picker is a combobox, not a menu: it filters, so its rows are
  // options. Only the effort control above is a menu.
  await expect(page.getByRole("option", { name: GPT_5_6_SOL_CAPABILITY_NAME })).toBeVisible();
  // The picker opens on the provider in force and lists only its models, so reaching another
  // provider's model is a tab away or a query away. Typing is the path a reader takes when
  // they already know the name, and it is the one that has to cross every tab.
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

    // The measure is the point: the body does not resize with its group, so the surface
    // cannot walk up the screen. Settle first — the popover enters at `scale(0.97)`, so a box
    // read mid-transition is 97% of the answer.
    const body = surface.locator("button[aria-pressed]").first().locator("..").locator("..");
    await expectStableBox(body);
    const before = await body.boundingBox();

    await rail.last().click();
    // Focus stays where it was used. The caret goes to the search on OPEN; re-running that on
    // every group change took it away from the rail, so a keyboard reader could never stay
    // there to try a second group.
    await expect(rail.last()).toBeFocused();
    await expectStableBox(body);
    expect((await body.boundingBox())!.height).toBe(before!.height);

    // A query leaves the rail behind: the results are not scoped to one provider any more, so
    // the rail would be lying about what is listed.
    await page.getByPlaceholder("Search models…").fill("gpt");
    await expect(surface.locator("button[aria-pressed]")).toHaveCount(0);

    // Escape gives the query back before it gives up the surface. Proved here and not only in
    // the unit suite: keeping the popover open depends on stopping the key before Base UI's
    // dismiss layer sees it, and that layer is a real one only in a real browser.
    await page.keyboard.press("Escape");
    await expect(surface).toBeVisible();
    await expect(page.getByPlaceholder("Search models…")).toHaveValue("");
    await expect(rail.first()).toBeVisible();

    await rail.first().click();

    // `data-[highlighted]:bg-hover` is the only feedback a row has, and the attribute behind
    // it is Base UI's to write — a variant keyed on a name the library does not set compiles,
    // ships and never matches. It needs a real pointer, so it is proved here rather than in a
    // unit test that can only synthesise one.
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

  await page.getByRole("button", { name: "Retry" }).click();

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

  const stream = page.locator(".msg-scroll > .panel-scroll");
  const overflow = await stream.evaluate((element) => element.scrollWidth - element.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
  await expect(page.locator('[data-slot="composer-root"]')).toBeVisible();
});

/**
 * Render every turn once, top to bottom.
 *
 * `content-visibility` holds an off-screen turn at its estimated height until it has
 * rendered, and `contain-intrinsic-size: auto` then remembers the real one — so the
 * transcript's total height depends on which turns happened to get rendering time. A golden
 * settles STABLY at a different offset run to run, which is why asserting the resting scroll
 * position cannot catch it.
 *
 * It is not only the goldens. Anything reaching INTO a turn needs the same pass: a lazy image
 * inside an unrendered subtree has no box, so `scrollIntoView` aims at nothing, the image
 * never loads, and the control around it reports itself hidden.
 */
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

/**
 * Wait until a locator's box stops moving.
 *
 * Playwright's `hover()` reads the box and then moves the pointer, so anything still easing
 * — the transcript's own scroll, an image resolving its size — is a pointer aimed where the
 * target used to be. The failure that follows is a control that never reveals, which reads
 * as a broken affordance rather than as a race.
 */
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
  // Settle the artwork BEFORE hovering: the image loading resizes the artifact, and a hover
  // aimed at where it used to be leaves the pointer outside it, so the control never reveals.
  await expect
    .poll(() => svgPreview.evaluate((image: HTMLImageElement) => image.naturalWidth))
    .toBe(240);
  // `toBeVisible` may scroll the artifact underneath the pointer left by the
  // earlier wrap click. Move it away so this measures the true resting state.
  await page.mouse.move(0, 0);
  await expect.poll(() => svgCopy.evaluate((button) => getComputedStyle(button).opacity)).toBe("0");
  // And settle where it is before aiming at it. `hover()` reads the box, then moves the
  // pointer — the transcript eases its own scroll, so under load the artifact has slid on by
  // the time the pointer arrives and the hover lands on nothing.
  await expectStableBox(svgArtifact);
  await svgArtifact.hover();
  await expect.poll(() => svgCopy.evaluate((button) => getComputedStyle(button).opacity)).toBe("1");
  await expect(svgArtifact.locator('[data-slot="shiki-preview-body"]')).toHaveAttribute(
    "tabindex",
    "0",
  );
});

test("code blocks use the Codex caption and source geometry", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=long-content");
  await page.locator("html[data-visual-ready]").waitFor();

  const block = page.locator(".shiki-block").filter({ hasText: "Execute(context.Context" });
  // Highlighting is async, so `.shiki` and the caption arrive after the ready flag. Reading
  // before they do returns null and fails on a frame rather than on the geometry.
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
    // Mermaid lays its own labels out in SVG and does not place their glyphs at the same
    // subpixel offset twice, which costs about two hundred pixels of text edge per run. The
    // rest of the suite holds a far tighter budget; this is the one golden that cannot.
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
    await expect(page.getByRole("dialog", { name: "Diagram" })).toBeVisible();
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
    await expect(preview.locator("img")).toHaveAttribute("loading", "lazy");
    await layOutTranscript(page);
    await preview.evaluate((button) => button.parentElement?.scrollIntoView({ block: "center" }));
    await expect(preview).toBeVisible();
    await expectStableBox(preview);
    await expect(preview).toHaveScreenshot(`markdown-image-${theme}.png`);
    await preview.click();
    const dialog = page.getByRole("dialog", { name: "Inline architecture" });
    await expect(dialog).toBeVisible();
    // The dialog is fixed but its 90% backdrop deliberately preserves the
    // transcript behind it. Pin that background before the golden: the
    // transcript's follow animation and `scrollIntoView` otherwise race over
    // which equally valid 49px slice shows through the backdrop.
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
  await expect(compaction.locator("xpath=..").locator(".h-px")).toHaveCount(0);
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

// The three seams around the reading plane are one primitive, and the top one is the
// easy one to lose: half a device pixel, so the raster comparison can pass on its
// absence, and the bars sit in their region's own colour with the body scrolling
// under them — with no seam the session title and the first line of a message share
// one field of white.
// Assert the shared mechanism so every chrome bar in the same visual row receives the
// same seam contract.
test("every chrome bar that takes a bottom edge wears the style edge", async ({ page }) => {
  await page.goto("/visual/?fixture=workspace&theme=light&state=dock-light");
  await page.locator("html[data-visual-ready]").waitFor();

  const measured = await page.evaluate(() => {
    // Resolve the expected value THROUGH the engine rather than composing the two
    // token strings: computed `box-shadow` is normalised (`rgba(0, 0, 0, 0.2)`,
    // `0px`) and the tokens are not, so a string built here would only ever assert
    // that this test can reproduce Chromium's serialiser.
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
  // A bar that already butts against another region takes nothing.
  for (const shadow of measured.withoutEdge) expect(shadow).toBe("none");
});

// The input rung floats over the transcript, so the transcript has to end above it.
// Nothing else can catch this: the tail is only reachable at full scroll, the
// overlap looks plausible on a fixture that fits its viewport, and the reservation
// is published by a ResizeObserver rather than written in a class — so it can be
// silently zero and every other assertion still passes.
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
        // The margin the contract adds on top of the panel's own height, read
        // rather than restated: `COMPOSER_CLEARANCE` guarantees this `1rem`
        // after its scroll-rounding guard, and a literal here would have to be
        // kept in step with a class in another file.
        margin: Math.round(Number.parseFloat(getComputedStyle(document.documentElement).fontSize)),
      };
    }, inputSurface);

    // Not merely positive: a tail resting against the surface edge is visually
    // crowded and can remain behind the composer's translucent material.
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

// Every state collapses its tool calls into an "N steps" summary, so until this
// test the rows themselves — the app's most-read surface — appeared in no
// screenshot and in no browser assertion. What it pins is what a row REPORTS: the
// subject it acted on, and for an edit the lines it changed.
// The plan was on screen twice: the active surface above the composer, and the
// tool row that wrote it. Nothing about that is visible to a golden — both readings look
// deliberate — so the assertion is that the transcript does not narrate a call whose
// surface already holds it.
test("a tool with a standing surface is not narrated as well", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=running");
  await page.locator("html[data-visual-ready]").waitFor();

  // The composer-owned pill holds the plan in its Codex-style hover surface.
  await page.getByRole("button", { name: "Step 2 / 3" }).hover();
  await expect(page.getByText("Review visual evidence", { exact: true })).toBeVisible();

  const stream = page.locator(".msg-scroll-viewport");
  // The transcript does not repeat it, closed or open.
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
  // Its rendered label, not the tool name — the row shows "Update the plan".
  await expect(stream.getByText("Update the plan")).toHaveCount(0);

  // The calls it does narrate are still there — the filter removed one row, not the run.
  await expect(stream.getByText("atomicity_and_idempotency.go").first()).toBeVisible();
});

// The frame every turn passes through: the answer's item is open and still empty.
// Nothing may be folded here — an empty block is not an answer, and treating it as one
// collapsed the thinking to a one-line row with nothing in it, while the reply it
// deferred to had not written a character.
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
  // Its body, not just its summary row.
  await expect(thinking).toContainText("The framework must expose execution capability");

  // And the live work is still a list of steps rather than one folded wave.
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

  // The point of the split, in a real layout: the path is too long for the row, so
  // the DIRECTORY is the part that gets clipped and the filename is whole. Measured
  // rather than screenshotted because it is the overflow that matters, and a golden
  // cannot tell "clipped on the left" from "clipped on the right" without a human.
  const clipping = await row.evaluate((element) => {
    // The visual fixture has a deliberate 1120px minimum canvas. Constrain this
    // production row itself to exercise the dock/composer-narrowing case without
    // replacing the app layout with a test-only viewport implementation.
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
  // The directory gives way FIRST and gives way further — that is the ordering this
  // pins, and it is the whole point of the atom. "The filename is never touched" is a
  // stronger claim than the layout can keep: a name wider than its column must
  // ellipsize rather than push the row past its container. It remains whole in the DOM
  // and in the title.
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

  // One h1, and it names the session rather than sitting inside it.
  const roots = outline.filter((heading) => heading.level === 1);
  expect(roots).toHaveLength(1);
  expect(roots[0]?.text).toBe("Agent · narrative");

  // Every turn is its child, and nothing a model wrote outranks the turn holding it.
  expect(
    outline.filter((heading) => heading.level === 2 && !heading.authored).length,
  ).toBeGreaterThan(0);
  for (const heading of outline) {
    if (heading.authored) expect.soft(heading.level).toBeGreaterThanOrEqual(3);
  }

  // No rung is skipped, which is the whole reason the body opens at h3 and not h4.
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

  const header = page.locator("[data-slot=agent-activity-disclosure] .sticky").first();
  await expect(header).toBeVisible();

  // Measured, not screenshotted: a golden of a scrolled transcript cannot tell
  // "the header stuck" from "the header happened to be in frame". The card's own
  // `overflow` decides this — `hidden` would make the card the scrollport and the
  // header would leave with its rows.
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
  // `hidden` here is the bug this guards: it silently turns the card into the
  // scrollport, and sticky then has nothing to stick to.
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
    // `delegated` has no frame golden. Its transcript renders two ways — the block lands a
    // pixel apart and every glyph in the frame differs by 9-11k pixels, deterministic in
    // magnitude and not in which one appears. Eight causes were measured and ruled out:
    // scroll position, the transcript's mask, element geometry, font readiness, resolving
    // `content-visibility` (which moved twenty-six other goldens and fixed nothing), the Vite
    // transform cache, the runner's within-file parallelism, and cropping to the scroller.
    // A budget wide enough to pass would be wider than a whole button, so the PAGE frame is
    // given up rather than the suite's sensitivity — the element frame below covers what the
    // state is named for, and its behaviour already was.
    if (state === "delegated") continue;
    test(`agent golden ${theme} ${state}`, async ({ page }) => {
      await page.goto(`/visual/?fixture=agent&theme=${theme}&state=${state}`);
      await page.locator("html[data-visual-ready]").waitFor();
      if (state === "long-content") {
        await expect(page.locator(".shiki-block .shiki")).toHaveCount(3);
        await expect(page.getByRole("img", { name: "Diagram" })).toBeVisible();
      }
      // The canonical tool-shell frame exists to photograph the tool grammar,
      // so open its completed wave before capturing it. A collapsed "6 steps"
      // row cannot catch icon, status, grouping or preview regressions in the
      // components the state is named for.
      if (state === "tool-shells") {
        await page.getByRole("button", { name: /steps/ }).first().click();
        await page
          .locator('[data-tool="apply_patch"] button[aria-expanded]')
          .filter({ hasText: "specialisedPreviewProjections.ts" })
          .click();
      }
      await layOutTranscript(page);

      // Put the transcript where it belongs BEFORE the clock stops, rather than
      // waiting to see where it lands. Ready only means the tree is mounted;
      // use-stick-to-bottom then eases the scroll with Date.now(), and the
      // resting position it eases toward moves under it — `content-visibility`
      // gives off-screen blocks an estimated height until they are laid out, so
      // the same transcript settles a pixel apart between two runs and every
      // row in the frame shifts with it. Every fixture sticks to the bottom, and
      // the bottom is a hard stop the browser clamps to: assert it.
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

      await expect(page).toHaveScreenshot(`agent-${theme}-${state}.png`);
    });
  }
}

// The ninth cause, which the page frame could not escape and an element frame does not have
// to: the shift is a WHOLE pixel, so a clip taken relative to the card's own box carries
// identical content at an identical raster phase. Bucketed 12 loads to one hash before it was
// written down. This is the delegated card — a sub-agent's own run, with its status, its step
// count and its nested delegation — which is the whole of what the state is named for.
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

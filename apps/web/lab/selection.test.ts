import { describe, expect, it } from "vitest";
import { StoryCatalog } from "./catalog";
import {
  LAB_INSTANCE_QUERY,
  LAB_PATH,
  previewURL,
  previewWidth,
  readSelection,
  selectStory,
  selectionSearch,
  failureSelection,
  fixtureColors,
  recoveryURL,
  type LabSelection,
} from "./selection";
import type { ComponentStory, FixtureStory, LabFixtureTarget, LabStory } from "./story";

function stories() {
  const foundation: ComponentStory = {
    kind: "component",
    panel: "foundation",
    id: "button-sample",
    title: { en: "Button", "zh-CN": "按钮" },
    summary: { en: "Local command interaction", "zh-CN": "本地命令交互" },
    group: { en: "Controls", "zh-CN": "控件" },
    source: "packages/ui/src/button.tsx",
    width: 480,
    height: 280,
    variants: [
      { id: "normal", title: { en: "Normal", "zh-CN": "常规" } },
      { id: "disabled", title: { en: "Disabled", "zh-CN": "禁用" } },
    ],
    load: async () => ({ default: () => null }),
  };
  const business: ComponentStory = {
    ...foundation,
    panel: "business",
    id: "status-sample",
    title: { en: "Run status", "zh-CN": "运行状态" },
    source: "apps/web/src/ui/agent/status-pill.tsx",
    variants: [{ id: "running", title: { en: "Running", "zh-CN": "运行中" } }],
  };
  const fixture: FixtureStory = {
    kind: "fixture",
    panel: "business",
    id: "workspace-sample",
    title: { en: "Workspace", "zh-CN": "工作区" },
    summary: { en: "Recorded workspace fixture", "zh-CN": "工作区记录场景" },
    group: { en: "Scenarios", "zh-CN": "场景" },
    source: "apps/web/lab/scenarios/installVisualWorkspaceFixture.ts",
    width: 1440,
    height: 900,
    variants: [
      {
        id: "settings",
        title: { en: "Appearance settings", "zh-CN": "外观设置" },
        target: { fixture: "workspace", state: "settings", pane: "appearance" },
      },
    ],
  };
  return {
    foundation,
    business,
    fixture,
    catalog: new StoryCatalog([foundation, business, fixture]),
  };
}

describe("component lab selection boundary", () => {
  it("recovers only valid appearance without changing or admitting the invalid request", () => {
    const { catalog } = stories();
    const query = new URLSearchParams(
      "story=missing&variant=missing&fixture=unknown&locale=en&theme=dark&motion=full&font-size=18",
    );
    const original = query.toString();
    const appearance = failureSelection(query);
    expect(appearance).toEqual({ locale: "en", theme: "dark", motion: "full", fontSize: 18 });
    expect(query.toString()).toBe(original);
    expect(() => readSelection(original, catalog)).toThrow(/unknown/i);
    const recovered = new URL(recoveryURL(appearance), "http://lab.test");
    expect([...recovered.searchParams.keys()].sort()).toEqual([
      "font-size",
      "locale",
      "motion",
      "theme",
    ]);
    expect(readSelection(recovered.search, catalog)).toMatchObject(appearance);
  });

  it("keeps the distinct Lab and direct fixture locale and font defaults", () => {
    expect(failureSelection(new URLSearchParams())).toEqual({
      locale: "zh-CN",
      theme: "light",
      motion: "off",
      fontSize: 14,
    });
    expect(failureSelection(new URLSearchParams(), "fixture").locale).toBe("en");
    expect(failureSelection(new URLSearchParams("locale=zh"), "fixture").locale).toBe("zh-CN");
    expect(failureSelection(new URLSearchParams("locale=de"), "fixture").locale).toBe("en");
    expect(failureSelection(new URLSearchParams("font-size=14.5")).fontSize).toBe(14);
    expect(failureSelection(new URLSearchParams("font-size=14.5"), "fixture").fontSize).toBe(15);
  });

  it("uses safe recovery appearance while strict selection still rejects illegal values", () => {
    const { catalog } = stories();
    const query = new URLSearchParams("locale=de&theme=system&motion=on&font-size=999");
    const appearance = failureSelection(query);
    expect(appearance).toEqual({ locale: "zh-CN", theme: "light", motion: "off", fontSize: 14 });
    expect(() => readSelection(query.toString(), catalog)).toThrow(/invalid/i);
    expect(
      readSelection(new URL(recoveryURL(appearance), "http://lab.test").search, catalog),
    ).toMatchObject(appearance);
  });

  it("preserves fixture color admission, clamping and complete custom palettes", () => {
    const query = new URLSearchParams({
      accent: "#6f42c1",
      contrast: "150",
      "custom-bg": "#102030",
      "custom-fg": "#eeeeee",
    });
    const original = query.toString();
    expect(fixtureColors(query)).toEqual({
      accent: "#6f42c1",
      contrast: 100,
      theme: "custom",
      customTheme: { bg: "#102030", fg: "#eeeeee" },
    });
    expect(query.toString()).toBe(original);
    expect(
      fixtureColors(new URLSearchParams("accent=red&contrast=invalid&custom-bg=%23102030")),
    ).toEqual({});
    expect(fixtureColors(new URLSearchParams("contrast=-5"))).toEqual({ contrast: 0 });
    expect(fixtureColors(new URLSearchParams("contrast="))).toEqual({ contrast: 0 });
  });

  it("selects each panel's own first story and a valid initial variant", () => {
    const { foundation, business, catalog } = stories();
    expect(readSelection("", catalog)).toEqual({
      storyId: foundation.id,
      variantId: foundation.variants[0].id,
      theme: "light",
      locale: "zh-CN",
      motion: "off",
      fontSize: 14,
      viewport: "component",
      mode: "gallery",
      compare: false,
    });
    expect(readSelection("?panel=business", catalog)).toMatchObject({
      storyId: business.id,
      variantId: business.variants[0].id,
    });
    expect(readSelection(`?story=${business.id}`, catalog).storyId).toBe(business.id);
  });

  it("roundtrips the complete selection through a canonical link", () => {
    const { fixture, catalog } = stories();
    const selection: LabSelection = {
      storyId: fixture.id,
      variantId: fixture.variants[0].id,
      theme: "dark",
      locale: "en",
      motion: "full",
      fontSize: 18,
      viewport: "tablet",
      mode: "inspect",
      compare: true,
    };
    const search = selectionSearch(selection, catalog);
    expect(new URLSearchParams(search).get("panel")).toBe("business");
    expect(readSelection(search, catalog)).toEqual(selection);
  });

  it.each([
    "panel=unknown",
    "story=missing",
    "story=button-sample&variant=missing",
    "panel=business&story=button-sample",
    "panel=foundation&story=status-sample",
    "theme=system",
    "locale=zh",
    "motion=on",
    "viewport=wide",
    "mode=edit",
    "compare=true",
    "variant=",
    "font-size=10",
    "font-size=19",
    "font-size=14.5",
    "font-size=",
    "font-size=not-a-size",
  ])("refuses an illegal URL selection: %s", (query) => {
    expect(() => readSelection(query, stories().catalog)).toThrow(/invalid|unknown|belong/i);
  });

  it("changes story and variant together while preserving inspection preferences", () => {
    const { business, catalog } = stories();
    const initial = readSelection("variant=disabled&theme=dark&mode=inspect&compare=on", catalog);
    const selected = selectStory(initial, business);
    expect(selected).toEqual({ ...initial, storyId: business.id, variantId: "running" });
    expect(initial.storyId).toBe("button-sample");
    expect(initial.variantId).toBe("disabled");
    expect(readSelection(selectionSearch(selected, catalog), catalog)).toEqual(selected);
  });
});

describe("preview document URLs", () => {
  it("preserves component locale and state while keeping sizing outside document identity", () => {
    const { foundation, catalog } = stories();
    const selection = readSelection("variant=disabled&theme=dark", catalog);
    const original = previewURL(foundation, selection, "button:instance-1");
    const resized = {
      ...selection,
      viewport: "phone" as const,
      mode: "inspect" as const,
      compare: true,
    };
    expect(previewURL(foundation, resized, "button:instance-1")).toBe(original);
    expect(previewWidth(foundation, "component")).toBe(480);
    expect(previewWidth(foundation, "phone")).toBe(390);
    const url = new URL(original, "https://flame.test");
    expect(url.pathname).toBe(`${LAB_PATH}preview.html`);
    expect(Object.fromEntries(url.searchParams)).toEqual({
      theme: "dark",
      locale: "zh-CN",
      motion: "off",
      "font-size": "14",
      [LAB_INSTANCE_QUERY]: "button:instance-1",
      story: foundation.id,
      variant: "disabled",
    });
  });

  it.each([
    ["zh-CN", "zh"],
    ["en", "en"],
  ] as const)(
    "maps lab locale %s to fixture locale %s at the existing fixture boundary",
    (locale, fixtureLocale) => {
      const { fixture, catalog } = stories();
      const selection = readSelection(`story=${fixture.id}&locale=${locale}&motion=full`, catalog);
      const url = new URL(previewURL(fixture, selection, "fixture:1"), "https://flame.test");
      expect(url.pathname).toBe(`${LAB_PATH}fixture.html`);
      expect(Object.fromEntries(url.searchParams)).toEqual({
        theme: "light",
        locale: fixtureLocale,
        motion: "full",
        "font-size": "14",
        [LAB_INSTANCE_QUERY]: "fixture:1",
        fixture: "workspace",
        state: "settings",
        pane: "appearance",
      });
    },
  );

  it.each<LabFixtureTarget>([
    { fixture: "agent", state: "empty" },
    { fixture: "shell", state: "loading" },
    { fixture: "workspace", state: "dock-review" },
  ])("projects the target into the existing $fixture fixture contract", (target) => {
    const { fixture, catalog } = stories();
    const configured: FixtureStory = {
      ...fixture,
      variants: [{ ...fixture.variants[0], target }],
    };
    const selection = readSelection(`story=${fixture.id}`, catalog);
    const url = new URL(previewURL(configured, selection, "fixture:1"), "https://flame.test");
    expect(url.searchParams.get("fixture")).toBe(target.fixture);
    expect(url.searchParams.get("state")).toBe(target.state);
    expect(url.searchParams.has("pane")).toBe(false);
  });

  it("refuses a fixture variant that has no canonical target", () => {
    const { fixture, catalog } = stories();
    const selection = readSelection(`story=${fixture.id}`, catalog);
    expect(() => previewURL(fixture, { ...selection, variantId: "missing" }, "fixture:1")).toThrow(
      /unknown fixture variant/i,
    );
  });
});

describe("story catalog ownership", () => {
  it("searches the requested panel in its selected locale and source vocabulary", () => {
    const { foundation, business, catalog } = stories();
    expect(catalog.search("foundation", "BUTTON disabled", "en")).toEqual([foundation]);
    expect(catalog.search("foundation", "按钮 禁用", "zh-CN")).toEqual([foundation]);
    expect(catalog.search("business", "status-pill", "en")).toEqual([business]);
    expect(catalog.search("business", "按钮", "zh-CN")).toEqual([]);
  });

  it("rejects duplicate identities, invalid geometry and unsafe source locations", () => {
    const { foundation, business } = stories();
    const invalid: LabStory[] = [
      { ...foundation, id: "../escape" },
      { ...foundation, variants: [foundation.variants[0], foundation.variants[0]] },
      { ...foundation, width: 0 },
      { ...foundation, height: Infinity },
      { ...foundation, source: "../outside.tsx" },
      { ...foundation, source: "https://elsewhere.test/component.tsx" },
    ];
    for (const story of invalid) {
      expect(() => new StoryCatalog([story, business])).toThrow(/invalid|duplicate/i);
    }
    expect(() => new StoryCatalog([foundation, foundation, business])).toThrow(/duplicate/i);
    expect(() => new StoryCatalog([foundation])).toThrow(/empty component panel/i);
  });

  it("keeps admitted identities valid after callers change their original descriptors", () => {
    const { foundation, business } = stories();
    const variant = { id: "admitted", title: { en: "Admitted", "zh-CN": "已登记" } };
    const descriptor: ComponentStory = { ...foundation, variants: [variant] };
    const catalog = new StoryCatalog([descriptor, business]);
    Reflect.set(variant, "id", "../changed-after-validation");
    Reflect.set(descriptor, "id", business.id);
    expect(catalog.story(foundation.id).variants[0].id).toBe("admitted");
    expect(readSelection(`story=${foundation.id}&variant=admitted`, catalog)).toMatchObject({
      storyId: foundation.id,
      variantId: "admitted",
    });
  });
});

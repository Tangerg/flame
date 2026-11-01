import { useEffect } from "react";
import { act, cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { createTestClient } from "@/test/renderClient";
import { contributeForTest, resetKernelForTest } from "@/test/kernel";
import { tupleKey } from "@/lib/tupleKey";
import { SHARED_CLIENT_EFFECT, SharedEffects } from "./SharedEffects";
import { resolveRenderer } from "@/features/plugins/public/renderers";

afterEach(async () => {
  cleanup();
  await resetKernelForTest();
});

it("switches an exclusive effect without retaining the other installation's subscription", async () => {
  const client = createTestClient();
  const subscriptions = new Set<string>();
  function candidate(id: string) {
    return {
      group: "notifications",
      id,
      title: id,
      component: function NotificationEffect() {
        useEffect(() => {
          subscriptions.add(id);
          return () => {
            subscriptions.delete(id);
          };
        }, []);
        return <span>{id}</span>;
      },
    };
  }
  try {
    await contributeForTest((ctx) => {
      for (const id of ["second", "first"])
        ctx.contribute(SHARED_CLIENT_EFFECT, candidate(id), { key: id });
    });
    const view = client.render(<SharedEffects />);
    expect(screen.getByText("first")).toBeTruthy();
    expect([...subscriptions]).toEqual(["first"]);
    const preferences = client.installedPlugins.access.presentationPreferences;
    const key = tupleKey(preferences.endpoint(), "effect", "notifications");
    act(() => preferences.choose(key, "second"));
    expect([...subscriptions]).toEqual(["second"]);
    act(() => preferences.choose(key, "no-longer-installed"));
    expect(subscriptions.size).toBe(0);
    act(() => preferences.choose(key, null));
    expect([...subscriptions]).toEqual(["first"]);
    view.unmount();
    expect(subscriptions.size).toBe(0);
  } finally {
    await client.dispose();
  }
});

it("keeps a chosen renderer bound to its installation through reordering and withdrawal", () => {
  const a = { id: "a", title: "Markdown" },
    b = { id: "b", title: "Markdown" };
  expect(resolveRenderer([a, b], null)).toBeUndefined();
  expect(resolveRenderer([b, a], "a")).toBe(a);
  expect(resolveRenderer([b], "a")).toBeUndefined();
  expect(resolveRenderer([b], null)).toBe(b);
});

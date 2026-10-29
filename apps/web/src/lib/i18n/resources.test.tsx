import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, screen } from "@testing-library/react";
import { useMemo } from "react";
import { afterEach, expect, it } from "vitest";
import { activeLocale, mountLocaleBundle, setLocale, useT } from "@/lib/i18n/index";

afterEach(() => {
  cleanup();
  setLocale("en");
});

function ReadingContext() {
  const t = useT();
  return <p>{t("common.loading")}</p>;
}

function MemoizedReadingContext() {
  const t = useT();
  const text = useMemo(() => t("common.loading"), [t]);
  return <p>{text}</p>;
}

it("updates mounted readers when the selected dictionary arrives and when its source withdraws", async () => {
  setLocale("fr-CA");
  testClient.render(<ReadingContext />);
  expect(screen.getByText("Loading")).toBeTruthy();
  let unmount!: () => void;
  try {
    act(() => {
      unmount = mountLocaleBundle("fr-CA", { "common.loading": "Lecture en attente" });
    });
    expect(await screen.findByText("Lecture en attente")).toBeTruthy();
    expect(activeLocale()).toBe("fr-CA");
  } finally {
    act(() => unmount?.());
  }
  expect(await screen.findByText("Loading")).toBeTruthy();
  expect(activeLocale()).toBe("fr-CA");
});

it("updates memoized copy on locale selection, dictionary withdrawal and later arrival", async () => {
  let unmount = mountLocaleBundle("fr-CA", { "common.loading": "Lecture en attente" });
  try {
    testClient.render(<MemoizedReadingContext />);
    expect(screen.getByText("Loading")).toBeTruthy();
    act(() => setLocale("fr-CA"));
    expect(await screen.findByText("Lecture en attente")).toBeTruthy();
    act(() => unmount());
    expect(await screen.findByText("Loading")).toBeTruthy();
    act(() => {
      unmount = mountLocaleBundle("fr-CA", { "common.loading": "Lecture arrivée" });
    });
    expect(await screen.findByText("Lecture arrivée")).toBeTruthy();
    expect(activeLocale()).toBe("fr-CA");
  } finally {
    act(() => unmount());
  }
});

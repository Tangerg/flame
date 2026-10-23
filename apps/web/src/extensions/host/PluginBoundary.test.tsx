import type React from "react";
import { useLayoutEffect, useRef } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { PluginBoundary } from "@/extensions/host/PluginBoundary";
import { usePluginErrorStore } from "@/extensions/errors";
function Boom(): React.ReactNode {
  throw new Error("kaboom");
}

describe("pluginBoundary", () => {
  it("renders children when nothing throws", () => {
    render(
      <PluginBoundary plugin="ok.plugin">
        <span>hello</span>
      </PluginBoundary>,
    );
    expect(screen.getByText("hello")).toBeTruthy();
  });

  it("catches a child render error and shows the default fallback", () => {
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    render(
      <PluginBoundary plugin="bad.plugin" label="Bad Plugin">
        <Boom />
      </PluginBoundary>,
    );
    expect(screen.getByText("Bad Plugin")).toBeTruthy();
    expect(screen.getByText(/failed to render/i)).toBeTruthy();
    expect(screen.getByText("kaboom").tagName).toBe("CODE");
    spy.mockRestore();
  });

  it("reports the error through the plugin error store with source=render", () => {
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    render(
      <PluginBoundary plugin="bad.plugin">
        <Boom />
      </PluginBoundary>,
    );
    const log = usePluginErrorStore.getState().log;
    expect(log).toHaveLength(1);
    expect(log[0]!.plugin).toBe("bad.plugin");
    expect(log[0]!.source).toBe("render");
    expect(log[0]!.message).toBe("kaboom");
    spy.mockRestore();
  });

  it("contains a failing pane and lets it try again without touching its neighbours", () => {
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    let fail = true;
    function Flaky(): React.ReactNode {
      if (fail) throw new Error("pane exploded");
      return <p>pane body</p>;
    }
    render(
      <>
        <PluginBoundary plugin="settings:flaky">
          <Flaky />
        </PluginBoundary>
        <p>neighbour</p>
      </>,
    );

    expect(screen.getByRole("alert").textContent).toContain("settings:flaky");
    expect(screen.getByText("neighbour")).toBeTruthy();

    fail = false;
    fireEvent.click(screen.getByRole("button", { name: /retry/i }));
    expect(screen.getByText("pane body")).toBeTruthy();
    spy.mockRestore();
  });

  describe("retry focus", () => {
    let fail: boolean;
    function Flaky(): React.ReactNode {
      if (fail) throw new Error("view exploded");
      return <p>recovered view</p>;
    }

    function focusRetry(): HTMLElement {
      const retry = screen.getByRole("button", { name: /retry/i });
      retry.focus();
      expect(document.activeElement).toBe(retry);
      return retry;
    }

    it("hands a focused Retry to its focusable host when the view recovers", () => {
      const spy = vi.spyOn(console, "error").mockImplementation(() => {});
      fail = true;
      render(
        <div role="region" aria-label="Diff" tabIndex={-1}>
          <PluginBoundary plugin="workspace:diff" label="Diff">
            <Flaky />
          </PluginBoundary>
        </div>,
      );

      const retry = focusRetry();
      fail = false;
      fireEvent.click(retry);

      expect(screen.getByText("recovered view")).toBeTruthy();
      expect(document.activeElement).toBe(screen.getByRole("region", { name: "Diff" }));
      spy.mockRestore();
    });

    it("keeps the host focused and the failure usable when retry fails again", () => {
      const spy = vi.spyOn(console, "error").mockImplementation(() => {});
      fail = true;
      render(
        <div role="region" aria-label="Diff" tabIndex={-1}>
          <PluginBoundary plugin="workspace:diff" label="Diff">
            <Flaky />
          </PluginBoundary>
        </div>,
      );

      fireEvent.click(focusRetry());

      expect(screen.getByRole("alert").textContent).toContain("view exploded");
      expect(document.activeElement).toBe(screen.getByRole("region", { name: "Diff" }));
      expect(usePluginErrorStore.getState().log).toHaveLength(2);

      const again = focusRetry();
      fail = false;
      fireEvent.click(again);
      expect(screen.getByText("recovered view")).toBeTruthy();
      spy.mockRestore();
    });

    it("chooses the nearest focusable host rather than an outer scope", () => {
      const spy = vi.spyOn(console, "error").mockImplementation(() => {});
      fail = true;
      render(
        <div role="region" aria-label="Workbench" tabIndex={-1}>
          <div role="region" aria-label="Diff" tabIndex={-1}>
            <PluginBoundary plugin="workspace:diff" label="Diff">
              <Flaky />
            </PluginBoundary>
          </div>
        </div>,
      );

      const retry = focusRetry();
      fail = false;
      fireEvent.click(retry);

      expect(document.activeElement).toBe(screen.getByRole("region", { name: "Diff" }));
      spy.mockRestore();
    });

    it("leaves focus that is already outside the failure untouched", () => {
      const spy = vi.spyOn(console, "error").mockImplementation(() => {});
      fail = true;
      render(
        <>
          <div role="region" aria-label="Diff" tabIndex={-1}>
            <PluginBoundary plugin="workspace:diff" label="Diff">
              <Flaky />
            </PluginBoundary>
          </div>
          <button type="button">Elsewhere</button>
        </>,
      );

      const elsewhere = screen.getByRole("button", { name: "Elsewhere" });
      elsewhere.focus();
      fail = false;
      fireEvent.click(screen.getByRole("button", { name: /retry/i }));

      expect(screen.getByText("recovered view")).toBeTruthy();
      expect(document.activeElement).toBe(elsewhere);
      spy.mockRestore();
    });

    it("lets recovered content claim focus for itself", () => {
      const spy = vi.spyOn(console, "error").mockImplementation(() => {});
      fail = true;
      function SelfFocusing(): React.ReactNode {
        const own = useRef<HTMLButtonElement>(null);
        useLayoutEffect(() => own.current?.focus(), []);
        if (fail) throw new Error("view exploded");
        return (
          <button ref={own} type="button">
            Recovered action
          </button>
        );
      }
      render(
        <div role="region" aria-label="Diff" tabIndex={-1}>
          <PluginBoundary plugin="workspace:diff" label="Diff">
            <SelfFocusing />
          </PluginBoundary>
        </div>,
      );

      const retry = focusRetry();
      fail = false;
      fireEvent.click(retry);

      expect(document.activeElement).toBe(screen.getByRole("button", { name: "Recovered action" }));
      spy.mockRestore();
    });

    it("still retries without inventing a destination when no focusable host exists", () => {
      const spy = vi.spyOn(console, "error").mockImplementation(() => {});
      fail = true;
      const { container } = render(
        <PluginBoundary plugin="workspace:diff" label="Diff">
          <Flaky />
        </PluginBoundary>,
      );

      const retry = focusRetry();
      fail = false;
      fireEvent.click(retry);

      expect(screen.getByText("recovered view")).toBeTruthy();
      expect(container.contains(document.activeElement)).toBe(false);
      spy.mockRestore();
    });
  });
});

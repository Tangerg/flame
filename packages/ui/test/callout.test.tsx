import { cleanup, render, screen } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, expect, it } from "vitest";
import {
  Callout,
  CalloutLabelsProvider,
  type CalloutLabels,
} from "../src/callout";

afterEach(() => cleanup());

const labels: CalloutLabels = {
  note: "Host note",
  tip: "Host guidance",
  important: "Host priority",
  warning: "Host warning",
  caution: "Host caution",
};

it("uses its host's labels and updates them without replacing the body", () => {
  const { rerender } = render(
    <CalloutLabelsProvider labels={labels}>
      <Callout kind="tip">
        <p>Original body.</p>
      </Callout>
    </CalloutLabelsProvider>,
  );
  const body = screen.getByText("Original body.");
  expect(screen.getByText("Host guidance")).toBeTruthy();
  rerender(
    <CalloutLabelsProvider labels={{ ...labels, tip: "Conseil du site" }}>
      <Callout kind="tip">
        <p>Original body.</p>
      </Callout>
    </CalloutLabelsProvider>,
  );
  expect(screen.getByText("Conseil du site")).toBeTruthy();
  expect(screen.getByText("Original body.")).toBe(body);
});

it("refuses a missing host-label contract", () => {
  expect(() =>
    renderToStaticMarkup(<Callout kind="note">Body.</Callout>),
  ).toThrow("callout labels provider is missing");
});

it("refuses an empty host label at its provider boundary", () => {
  expect(() =>
    renderToStaticMarkup(
      <CalloutLabelsProvider labels={{ ...labels, warning: " " }}>
        Body.
      </CalloutLabelsProvider>,
    ),
  ).toThrow("callout label is missing for warning");
});

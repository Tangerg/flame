import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { Button, PanelTitle, TextField } from "@flame/ui";

const components = {
  TITLE: PanelTitle,
  REFRESH: Button,
  FILTER: TextField,
  MORE: Button,
};

export function renderControls(controls) {
  return Object.fromEntries(
    Object.entries(controls).map(([slot, props]) => {
      const component = components[slot];
      if (!component) throw new Error(`unsupported renderer control: ${slot}`);
      return [slot, renderToStaticMarkup(createElement(component, props))];
    }),
  );
}

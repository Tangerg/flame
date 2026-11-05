// @vitest-environment happy-dom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { type ComponentContext } from "@flame/component-sdk";
import { Gallery } from "./component";

vi.mock("@flame/icons/catalog", () => ({
  icons: Object.freeze(
    [
      {
        name: "brain",
        component: "Brain",
        label: "Brain",
        tags: ["thinking", "reasoning"],
        group: "Development",
      },
      {
        name: "brain-off",
        component: "BrainOff",
        label: "Thinking off",
        tags: ["thinking", "reasoning"],
        group: "Development",
      },
      {
        name: "circle",
        component: "Circle",
        label: "Circle",
        tags: [],
        group: "Geometry",
      },
    ].map((icon) => Object.freeze(icon)),
  ),
}));
afterEach(cleanup);

const show = (locale: string) =>
  render(<Gallery context={{ locale } as unknown as ComponentContext} />);
const group = (name: string) =>
  within(screen.getByRole("heading", { name }).closest("section")!);

it.each([
  ["en", "1 icon", "2 icons", "Clear filter"],
  ["zh", "1 个图标", "2 个图标", "清除筛选"],
  ["zh-TW", "1 個圖示", "2 個圖示", "清除篩選"],
  ["ja", "1 個のアイコン", "2 個のアイコン", "絞り込みをクリア"],
  ["ko", "아이콘 1개", "아이콘 2개", "필터 지우기"],
  ["es", "1 icono", "2 iconos", "Borrar filtro"],
  ["fr", "1 icône", "2 icônes", "Effacer le filtre"],
  ["de", "1 Symbol", "2 Symbole", "Filter löschen"],
])("%s group counts follow plural grammar", (locale, one, other, clear) => {
  show(locale);
  group("Development").getByText(other);
  group("Geometry").getByText(one);

  const search = screen.getByRole("searchbox");
  fireEvent.change(search, { target: { value: "brain-off" } });
  group("Development").getByText(one);
  screen.getByText("BrainOff");
  screen.getByText("brain-off");
  expect(screen.queryByText("brain")).toBeNull();
  expect(screen.queryByRole("heading", { name: "Geometry" })).toBeNull();

  fireEvent.click(screen.getByRole("button", { name: clear }));
  expect(search).toHaveProperty("value", "");
  expect(document.activeElement).toBe(search);
  group("Development").getByText(other);
  group("Geometry").getByText(one);
});

it("keeps the total summary on the fixed catalog through empty results", () => {
  show("en");
  const search = screen.getByRole("searchbox");
  const status = screen.getByRole("status");
  expect(status.textContent).toBe("3 of 3 icons");
  fireEvent.change(search, { target: { value: "brain-off" } });
  expect(status.textContent).toBe("1 of 3 icons");
  fireEvent.change(search, { target: { value: "no such icon" } });
  expect(status.textContent).toBe("0 of 3 icons");
  screen.getByText("No matching icons");
  expect(screen.queryByRole("heading", { name: "Development" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Clear filter" }));
  expect(status.textContent).toBe("3 of 3 icons");
  group("Development").getByText("2 icons");
});

it("orders the French summary around the fixed total", () => {
  show("fr");
  fireEvent.change(screen.getByRole("searchbox"), {
    target: { value: "brain-off" },
  });
  expect(screen.getByRole("status").textContent).toBe("1 sur 3 icônes");
});

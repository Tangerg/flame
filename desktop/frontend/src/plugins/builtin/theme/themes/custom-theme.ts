import { colord } from "colord";
import { paletteThemePlugin } from "./paletteThemePlugin";

export default paletteThemePlugin("flame.builtin.custom-theme", ({ customTheme }) => ({
  id: "custom",
  label: "Custom",
  scheme: colord(customTheme.bg).isDark() ? "dark" : "light",
  icon: "palette",
  order: 99,
  palette: { background: customTheme.bg, foreground: customTheme.fg },
}));

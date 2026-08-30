import { SidebarExpanded } from "./SidebarExpanded";

// There is no collapsed "rail" variant — collapsing drops the column entirely.
export function SidebarPanel() {
  return <SidebarExpanded />;
}

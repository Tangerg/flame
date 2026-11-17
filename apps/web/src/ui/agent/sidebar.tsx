import { type ReactNode } from "react";
export function AgentSidebar({ label, children }: { label: string; children: ReactNode }) {
  return (
    <aside aria-label={label} className="agent-drawer" tabIndex={-1} data-focus-inset="">
      <div className="agent-drawer-surface">{children}</div>
    </aside>
  );
}

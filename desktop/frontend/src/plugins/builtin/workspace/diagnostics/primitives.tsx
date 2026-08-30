import type { ReactNode } from "react";
import { useRef } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useT } from "@/lib/i18n";

export function Empty({ hint }: { hint: string }) {
  const t = useT();
  return <div className="text-ui-md text-fg-faint">{t("diagnostics.empty", { hint })}</div>;
}

export function Row({
  children,
  head,
  className,
}: {
  children: ReactNode;
  head?: boolean;
  className?: string;
}) {
  return (
    <div
      className={
        "flex items-center gap-3 px-1 font-mono text-ui-md " +
        (head ? "text-ui-xs text-fg-faint" : "text-fg hover:bg-hover transition-colors") +
        (className ? " " + className : "")
      }
    >
      {children}
    </div>
  );
}

export function Cell({ className, children }: { className: string; children?: ReactNode }) {
  return <div className={`min-w-0 ${className}`}>{children}</div>;
}

export function VirtualList({
  count,
  rowHeight,
  header,
  renderRow,
}: {
  count: number;
  rowHeight: number;
  header: ReactNode;
  renderRow: (index: number) => ReactNode;
}) {
  const parentRef = useRef<HTMLDivElement>(null);
  const virt = useVirtualizer({
    count,
    getScrollElement: () => parentRef.current,
    estimateSize: () => rowHeight,
    overscan: 12,
  });

  return (
    <div className="flex flex-1 min-h-0 flex-col">
      {header}
      <div ref={parentRef} className="flex-1 min-h-0 overflow-y-auto">
        <div className="relative w-full" style={{ height: virt.getTotalSize() }}>
          {virt.getVirtualItems().map((vi) => (
            <div
              key={vi.key}
              data-index={vi.index}
              ref={virt.measureElement}
              className="absolute left-0 top-0 w-full"
              style={{ transform: `translateY(${vi.start}px)` }}
            >
              {renderRow(vi.index)}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

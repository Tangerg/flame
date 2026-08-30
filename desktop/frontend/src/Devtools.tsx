import { lazy, Suspense } from "react";
import type { AnyRouter } from "@tanstack/react-router";

const Panels = import.meta.env.DEV ? lazy(() => import("./devtoolsPanels")) : null;

export function Devtools({ router }: { router: AnyRouter }) {
  if (!Panels) return null;
  return (
    <Suspense>
      <Panels router={router} />
    </Suspense>
  );
}

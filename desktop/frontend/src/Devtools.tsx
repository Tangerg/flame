// Gated twice on purpose: `import.meta.env.DEV` folds to `false` in a build, so the ternary
// is dead code and the dynamic import is never emitted. That is what lets the three
// packages behind these panels stay devDependencies.

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

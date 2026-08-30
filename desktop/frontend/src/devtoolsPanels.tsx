import type { AnyRouter } from "@tanstack/react-router";
import { TanStackDevtools } from "@tanstack/react-devtools";
import { ReactQueryDevtoolsPanel } from "@tanstack/react-query-devtools";
import { TanStackRouterDevtoolsPanel } from "@tanstack/react-router-devtools";

export default function DevtoolsPanels({ router }: { router: AnyRouter }) {
  return (
    <TanStackDevtools
      plugins={[
        {
          id: "query",
          name: "Query",
          render: <ReactQueryDevtoolsPanel />,
        },
        {
          id: "router",
          name: "Router",
          render: <TanStackRouterDevtoolsPanel router={router} />,
        },
      ]}
    />
  );
}

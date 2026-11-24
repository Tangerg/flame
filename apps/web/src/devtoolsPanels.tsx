import { type AnyRouter } from "@tanstack/react-router";
import { TanStackDevtools } from "@tanstack/react-devtools";
import { ReactQueryDevtoolsPanel } from "@tanstack/react-query-devtools";
import { TanStackRouterDevtoolsPanel } from "@tanstack/react-router-devtools";
export default function DevtoolsPanels({ router }: { router: AnyRouter }) {
  return (
    <TanStackDevtools
      // The floating trigger sits on the composer's send button; the panel
      // still opens with its hotkey (Control+~).
      config={{ triggerHidden: true }}
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

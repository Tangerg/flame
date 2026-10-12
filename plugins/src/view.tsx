import {
  useSyncExternalStore,
  type ComponentType,
  type ReactNode,
} from "react";
import { createRoot } from "react-dom/client";
import { Refresh } from "@flame/icons/react";
import {
  Button,
  FontScaleProvider,
  IconButton,
  Metadata,
  Panel,
  PanelTitle,
  Toolbar,
  TooltipProvider,
  useProjectedFontSize,
} from "@flame/ui";
import { ReadView, type ReadState } from "./read-view";

export type ViewProps<T> = { view: ReadView<T> };
export function useRead<T>(view: ReadView<T>): ReadState<T> {
  return useSyncExternalStore(view.subscribe, view.getSnapshot);
}
export function ViewChrome({ children }: { children: ReactNode }) {
  const fontSize = useProjectedFontSize();
  return (
    <FontScaleProvider fontSize={fontSize}>
      <TooltipProvider>
        <main aria-label="Plugin content">{children}</main>
      </TooltipProvider>
    </FontScaleProvider>
  );
}
export function ViewPanel<T>({
  view,
  title,
  description,
  children,
}: ViewProps<T> & { title: string; description: string; children: ReactNode }) {
  const state = useRead(view);
  return (
    <Panel label={title}>
      <Toolbar label={`${title} overview`}>
        <PanelTitle>{title}</PanelTitle>
        <IconButton
          id="refresh"
          icon={Refresh}
          title="Refresh"
          disabled={state.pending !== undefined}
          onClick={() => view.read()}
        />
      </Toolbar>
      <Metadata>{description}</Metadata>
      {children}
    </Panel>
  );
}
export function ReadStatus<T>({
  view,
  ready,
  loading,
}: ViewProps<T> & { ready: string; loading: string }) {
  const state = useRead(view);
  return (
    <div id="status" role={state.error ? "alert" : "status"}>
      {state.error ?? (state.pending !== undefined ? loading : ready)}
    </div>
  );
}
export function Pagination<T>({
  view,
  nextCursor,
  label,
  previousLabel = "Previous page",
  nextLabel = "Next page",
}: ViewProps<T> & {
  nextCursor?: string;
  label: string;
  previousLabel?: string;
  nextLabel?: string;
}) {
  const state = useRead(view);
  const pending = state.pending !== undefined;
  return (
    <nav aria-label={label}>
      <Toolbar label={label}>
        <Button
          id="previous"
          size="sm"
          variant="outline"
          disabled={pending || !state.history.length}
          onClick={() => view.read(state.history.slice(0, -1))}
        >
          {previousLabel}
        </Button>
        <Button
          id="next"
          size="sm"
          variant="outline"
          disabled={pending || !nextCursor}
          onClick={() => {
            if (nextCursor) view.read([...state.history, nextCursor]);
          }}
        >
          {nextLabel}
        </Button>
      </Toolbar>
    </nav>
  );
}
export function mountView<T>(
  Component: ComponentType<ViewProps<T>>,
  validate?: (data: T) => void,
): void {
  function connect(event: MessageEvent) {
    if (
      event.source !== parent ||
      event.data?.type !== "flame.view.connect.v3" ||
      event.ports.length !== 1
    )
      return;
    removeEventListener("message", connect);
    const port = event.ports[0]!;
    let view: ReadView<T>;
    try {
      view = new ReadView<T>(event.data.initial, port, validate);
    } catch (error) {
      port.postMessage({
        type: "failure",
        reason:
          error instanceof Error ? error.message : "Unsupported page data.",
      });
      return;
    }
    createRoot(document.getElementById("root")!, {
      onUncaughtError: () =>
        port.postMessage({
          type: "failure",
          reason:
            "The plugin data could not be displayed. Refresh the page to try again.",
        }),
    }).render(
      <ViewChrome>
        <Component view={view} />
      </ViewChrome>,
    );
  }
  addEventListener("message", connect);
  parent.postMessage("flame.view.ready.v3", "*");
}

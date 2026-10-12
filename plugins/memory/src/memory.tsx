import { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { Refresh, Brain } from "@flame/icons/react";
import type { AgentMemoryItem, Page } from "@flame/runtime-contract/wire";
import {
  Badge,
  Button,
  Checkbox,
  EmptyState,
  FontScaleProvider,
  IconButton,
  ListRow,
  Metadata,
  Panel,
  PanelTitle,
  SearchField,
  Select,
  Toolbar,
  TooltipProvider,
  useProjectedFontSize,
} from "@flame/ui";

type Snapshot = {
  page: Page<AgentMemoryItem>;
  history: string[];
  pending?: string[];
  error?: string;
};

function MemoryView({
  initial,
  port,
}: {
  initial: Page<AgentMemoryItem>;
  port: MessagePort;
}) {
  const fontSize = useProjectedFontSize();
  const [snapshot, setSnapshot] = useState<Snapshot>({
    page: initial,
    history: [],
  });
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("all");
  const [origin, setOrigin] = useState("all");
  const [pinned, setPinned] = useState(false);
  useEffect(() => {
    const receive = (event: MessageEvent) => {
      const reply = event.data;
      if (reply?.type !== "data" && reply?.type !== "error") return;
      setSnapshot((current) => {
        if (!current.pending) return current;
        if (reply.type === "error")
          return { ...current, pending: undefined, error: reply.reason };
        return { page: reply.data, history: current.pending };
      });
    };
    port.addEventListener("message", receive);
    port.start();
    return () => port.removeEventListener("message", receive);
  }, [port]);

  const read = (history: string[]) => {
    if (snapshot.pending) return;
    setSnapshot({ ...snapshot, pending: history, error: undefined });
    port.postMessage({
      type: "read",
      ...(history.length ? { cursor: history.at(-1) } : {}),
    });
  };
  const rows = snapshot.page.data;
  const matching = rows.filter(
    (item) =>
      (status === "all" || item.status === status) &&
      (origin === "all" || item.origin === origin) &&
      (!pinned || item.pinned) &&
      `${item.id} ${item.content} ${item.sessionId ?? ""}`
        .toLocaleLowerCase()
        .includes(search.toLocaleLowerCase()),
  );
  const pending = snapshot.pending !== undefined;
  return (
    <FontScaleProvider fontSize={fontSize}>
      <TooltipProvider>
        <Panel label="Agent memory">
          <Toolbar label="Memory overview">
            <PanelTitle>Agent memory</PanelTitle>
            <IconButton
              id="refresh"
              icon={Refresh}
              title="Refresh"
              disabled={pending}
              onClick={() => read([])}
            />
          </Toolbar>
          <Metadata>
            Review proposed facts before they are recalled in future
            conversations. Manage memory with the controls above.
          </Metadata>
          <Toolbar label="Memory filters">
            <SearchField
              id="search"
              aria-label="Search this page"
              placeholder="Search this page"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
            <Select
              label="Memory status"
              value={status}
              onValueChange={setStatus}
              options={[
                { value: "all", label: "All statuses" },
                { value: "pending", label: "Pending review" },
                { value: "active", label: "Active" },
              ]}
            />
            <Select
              label="Memory origin"
              value={origin}
              onValueChange={setOrigin}
              options={[
                { value: "all", label: "All origins" },
                { value: "auto", label: "Mined" },
                { value: "user", label: "User authored" },
              ]}
            />
            <Checkbox
              checked={pinned}
              onCheckedChange={setPinned}
              label="Pinned only"
            />
          </Toolbar>
          <Metadata>
            <span id="summary">
              {rows.filter((item) => item.status === "pending").length} pending
              · {rows.filter((item) => item.status === "active").length} active
              on this page
            </span>
          </Metadata>
          <div id="status" role={snapshot.error ? "alert" : "status"}>
            {snapshot.error ??
              (pending
                ? "Loading recorded memory…"
                : `${rows.length} memory items loaded on this page.`)}
          </div>
          <main id="items">
            {!matching.length && (
              <EmptyState
                icon={Brain}
                size="compact"
                title={
                  rows.length
                    ? "No matching items on this page."
                    : "No memory items in this target."
                }
              />
            )}
            {matching.map((item) => (
              <ListRow
                key={item.id}
                data-memory-id={item.id}
                data-status={item.status}
              >
                <Badge tone={item.status === "pending" ? "accent" : "neutral"}>
                  {item.status === "pending" ? "Pending review" : "Active"}
                </Badge>
                <p>{item.content}</p>
                <Metadata>
                  {item.scope} ·{" "}
                  {item.origin === "auto" ? "Mined" : "User authored"}
                  {item.pinned ? " · Pinned" : ""}
                </Metadata>
                <Metadata>
                  Item: {item.id} · Created: {item.createdAt} · Updated:{" "}
                  {item.updatedAt}
                </Metadata>
                {item.sessionId && (
                  <Metadata>Session: {item.sessionId}</Metadata>
                )}
                {item.day && <Metadata>Day: {item.day}</Metadata>}
              </ListRow>
            ))}
          </main>
          <Toolbar label="Memory pages">
            <Button
              id="previous"
              variant="outline"
              disabled={pending || !snapshot.history.length}
              onClick={() => read(snapshot.history.slice(0, -1))}
            >
              Previous page
            </Button>
            <Button
              id="next"
              variant="outline"
              disabled={pending || !snapshot.page.nextCursor}
              onClick={() =>
                read([...snapshot.history, snapshot.page.nextCursor!])
              }
            >
              Next page
            </Button>
          </Toolbar>
        </Panel>
      </TooltipProvider>
    </FontScaleProvider>
  );
}

function connect(event: MessageEvent) {
  if (
    event.source !== parent ||
    event.data?.type !== "flame.view.connect.v3" ||
    event.ports.length !== 1
  )
    return;
  removeEventListener("message", connect);
  createRoot(document.getElementById("root")!).render(
    <MemoryView initial={event.data.initial} port={event.ports[0]!} />,
  );
}
addEventListener("message", connect);
parent.postMessage("flame.view.ready.v3", "*");

import { useWorkspace } from "@/features/workspace/public/workspace";
import * as stylex from "@stylexjs/stylex";
import {
  type ComponentProps,
  type KeyboardEvent,
  type ReactNode,
  type Ref,
  type RefObject,
} from "react";
import { ContentVisibility } from "@/ui/index";
import { AgentSurfaceHeader } from "@/ui/agent/index";
import { sessionDisplayTitle, useActiveSession } from "@/features/agent/public/session";
import {
  closeWorkspaceView,
  openWorkspaceViewInDock,
} from "@/features/workspace/public/navigation";
import { Slot } from "@/extensions/host/Slot";
import { ChatStream } from "@/features/shell/workbench/panel/ChatStream";
import { TranscriptReaderProvider } from "@/ui/agent/transcript-reader";
import { RuntimeConnectionNotice } from "@/features/shell/workbench/panel/RuntimeConnectionNotice";
import { readingColumn as rc } from "@/features/chat/public/message/rendering";
import { space } from "@flame/ui/tokens.stylex";
import { RunAnnouncer } from "@/features/shell/workbench/panel/RunAnnouncer";
import { RunStatusPill } from "@/features/shell/workbench/panel/RunStatusPill";
import { SessionIdentity } from "@/features/shell/workbench/panel/SessionIdentity";
import { WorkspaceWindowHeader } from "@/features/shell/workbench/panel/WorkspaceWindowHeader";
import { useT } from "@/lib/i18n/index";
import { shellStyles as sh } from "@/features/shell/workbench/shellStyles";
import { type AgentInput } from "@/features/agent/public/input";

function ConversationContent({
  ready,
  sessionId,
  onSend,
}: {
  ready: boolean;
  sessionId: string;
  onSend: (input: AgentInput) => boolean;
}) {
  const t = useT();
  const session = useActiveSession();
  if (!ready)
    return (
      <div {...stylex.props(rc.box, rc.gutter, paneStyles.pendingConnection)}>
        <RuntimeConnectionNotice />
      </div>
    );
  return (
    <>
      <AgentSurfaceHeader corner="window">
        <SessionIdentity
          sessionId={sessionId}
          title={session ? sessionDisplayTitle(t, session.title) : t("session.draft")}
          workspacePath={session?.workspace.path}
          detail={<RunStatusPill />}
        />
        <Slot name="chat.header.meta" />
      </AgentSurfaceHeader>
      <TranscriptReaderProvider sessionId={sessionId}>
        <Slot name="chat.search" />
        <ChatStream onSend={onSend} />
      </TranscriptReaderProvider>
      <RunAnnouncer />
    </>
  );
}

export function ConversationPane({
  ref,
  hidden,
  fallbackFocus,
  ready,
  sessionId,
  onSend,
}: {
  ref: Ref<HTMLDivElement>;
  hidden: boolean;
  fallbackFocus: RefObject<HTMLElement | null>;
  ready: boolean;
  sessionId: string;
  onSend: (input: AgentInput) => boolean;
}) {
  const t = useT();
  return (
    <div
      ref={ref}
      inert={hidden}
      aria-hidden={hidden || undefined}
      tabIndex={-1}
      role="region"
      aria-label={t("shell.region.conversation")}
      data-focus-inset=""
      {...stylex.props(sh.paneNarrow, paneStyles.conversation, hidden && paneStyles.inactiveView)}
    >
      <ContentVisibility visible={!hidden} fallbackFocus={fallbackFocus}>
        <ConversationContent ready={ready} sessionId={sessionId} onSend={onSend} />
      </ContentVisibility>
    </div>
  );
}

// An inspected full-pane view returns to its source; any other closes.
export function FullPaneHeader({
  viewId,
  view,
  returnAction,
  dockAvailable,
  onKeyDown,
  picker,
}: {
  viewId: string;
  view: ComponentProps<typeof WorkspaceWindowHeader>["view"];
  returnAction: { label: string; onReturn: () => void } | undefined;
  dockAvailable: boolean;
  onKeyDown: ((event: KeyboardEvent<HTMLElement>) => void) | undefined;
  picker: ReactNode;
}) {
  const workspaceAccess = useWorkspace();
  return (
    <WorkspaceWindowHeader
      view={view}
      exit={
        returnAction
          ? { kind: "return", ...returnAction }
          : { kind: "close", onClose: () => closeWorkspaceView(workspaceAccess, viewId) }
      }
      onOpenInDock={
        dockAvailable ? () => openWorkspaceViewInDock(workspaceAccess, viewId) : undefined
      }
      onKeyDown={onKeyDown}
    >
      {picker}
    </WorkspaceWindowHeader>
  );
}

export const paneStyles = stylex.create({
  conversation: {
    height: "100%",
    containerType: "inline-size",
    containerName: "conversation",
  },
  pendingConnection: { paddingBlock: space.s4 },
  // Removing layout makes WebKit reset the retained scrollport's reading position.
  inactiveView: { visibility: "hidden" },
});

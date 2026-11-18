import * as stylex from "@stylexjs/stylex";
import {
  useCompleteTranscriptRequired,
  usePublishTranscriptReader,
  type TranscriptReader,
} from "@/ui/agent/transcript-reader";
import { type BlockCtx } from "@/features/chat/public/message/rendering";
import { type TranscriptRow } from "@/features/agent/public/conversation";
import { useComposer } from "@/features/composer/public/composer";
import { useState } from "react";
import { useCurrentRootMaterial } from "@/features/agent/public/run";
import { MessageTranscript } from "@/features/chat/public/message/rendering";
import { RootRunOutcome } from "@/features/chat/public/message/rendering";
import { space } from "@flame/ui/tokens.stylex";
const ms = stylex.create({
  afterBlock: { marginTop: space.s4 },
});

interface Props {
  rows: readonly TranscriptRow[];
  ctx: BlockCtx;
  sessionId: string;
}

export function MessageStream({ rows, ctx, sessionId }: Props) {
  const currentRoot = useCurrentRootMaterial();
  const running = currentRoot.running;
  const terminalRow = rows[currentRoot.terminalTurnIndex(rows)];
  const [reader, setReader] = useState<TranscriptReader | null>(null);
  usePublishTranscriptReader(reader);
  const renderAll = useCompleteTranscriptRequired();
  const draftLoading = useComposer().useDraftLoading();

  return (
    <MessageTranscript
      className="msg-scroll-viewport"
      rows={rows}
      renderAll={renderAll}
      initialEndLayoutReady={!draftLoading || ctx.questionInComposer !== undefined}
      onFollowChange={(list, atEnd) =>
        setReader((current) =>
          current?.list === list && current.atEnd === atEnd ? current : { sessionId, list, atEnd },
        )
      }
      ctx={ctx}
      sessionId={sessionId}
      isRunning={running}
      terminalFooter={
        terminalRow
          ? {
              messageId: terminalRow.message.id,
              content: (
                <div {...stylex.props(ms.afterBlock)}>
                  <RootRunOutcome material={currentRoot} />
                </div>
              ),
            }
          : undefined
      }
    />
  );
}

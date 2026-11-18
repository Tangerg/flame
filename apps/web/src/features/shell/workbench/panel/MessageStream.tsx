import * as stylex from "@stylexjs/stylex";
import {
  useCompleteTranscriptRequired,
  usePublishTranscriptReader,
  type TranscriptReader,
} from "@/ui/agent/transcript-reader";
import { type BlockCtx } from "@/features/chat/public/message/rendering";
import { type TranscriptRow } from "@/features/agent/public/conversation";
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

  return (
    <MessageTranscript
      className="msg-scroll-viewport"
      key={sessionId}
      rows={rows}
      renderAll={renderAll}
      onFollowChange={(list, atEnd) =>
        setReader((current) =>
          current?.list === list && current.atEnd === atEnd ? current : { list, atEnd },
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

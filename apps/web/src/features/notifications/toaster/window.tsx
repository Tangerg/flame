import { ClipboardReceiptProvider } from "@flame/ui";
import { t } from "@/lib/i18n/index";
import { Toast } from "@base-ui/react/toast";
import { useLayoutEffect, useState, type ReactNode } from "react";
import { NotificationFeed } from "@/features/notifications/public/notifications";

const TOAST_TIMEOUT = 4000;

type StartWindow = (messages: NotificationFeed, render: (content: ReactNode) => void) => () => void;

export function WindowNotifications({ start }: { start: StartWindow }) {
  return (
    <Toast.Provider timeout={TOAST_TIMEOUT} limit={Infinity}>
      <NotificationRoot start={start} />
    </Toast.Provider>
  );
}

function NotificationRoot({ start }: { start: StartWindow }) {
  const { add, close } = Toast.useToastManager();
  const [content, render] = useState<ReactNode>(null);
  useLayoutEffect(() => {
    const messages = new NotificationFeed((message) => {
      const id = add({
        title: message.message,
        description: message.description,
        type: message.level,
        timeout: message.lifetime === "persistent" ? 0 : TOAST_TIMEOUT,
        actionProps: message.action && {
          children: message.action.label,
          onClick: () => {
            message.action!.run();
            close(id);
          },
        },
      });
    });
    let stop: () => void;
    try {
      stop = start(messages, render);
    } catch (error) {
      messages.dispose();
      throw error;
    }
    return () => {
      messages.dispose();
      stop();
    };
  }, [add, close, start]);
  return (
    <ClipboardReceiptProvider
      onReceipt={(receipt) =>
        add({
          id: receipt.id,
          type: receipt.status === "copied" ? "success" : "error",
          title: t(receipt.status === "copied" ? "common.copied" : "common.copyFailed"),
          description: receipt.label,
        })
      }
    >
      {content}
    </ClipboardReceiptProvider>
  );
}

import type { ReactNode } from "react";
import type { FallbackProps } from "react-error-boundary";
import { ErrorBoundary } from "react-error-boundary";
import { useT } from "@/lib/i18n";
import { Button, Well } from "@/ui";

interface Props {
  resetKey?: unknown;
  label?: string;
  children: ReactNode;
}

function ChatErrorFallback({ error, resetErrorBoundary }: FallbackProps) {
  const t = useT();
  return (
    <div role="alert" className="m-8 max-w-[720px] rounded-lg bg-negative-wash px-5 py-4 text-fg">
      <div className="mb-2 font-semibold text-display-sm tracking-tight text-negative">
        {t("chat.error.title")}
      </div>
      <Well cap="md" className="mb-3">
        {error instanceof Error ? error.message : String(error)}
      </Well>
      <div className="flex gap-2">
        <Button
          type="button"
          variant="soft"
          size="sm"
          onClick={resetErrorBoundary}
          className="rounded-md bg-canvas px-3 py-1 text-ui-md text-fg font-sans transition-colors hover:bg-surface-2"
        >
          {t("chat.error.retry")}
        </Button>
      </div>
    </div>
  );
}

export function ChatErrorBoundary({ resetKey, label, children }: Props) {
  return (
    <ErrorBoundary
      FallbackComponent={ChatErrorFallback}
      resetKeys={resetKey === undefined ? [] : [resetKey]}
      onError={(error, info) => {
        console.error(`[chat-error-boundary] ${label ?? "chat"}:`, error, info.componentStack);
      }}
    >
      {children}
    </ErrorBoundary>
  );
}

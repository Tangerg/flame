import { type ComponentType, type ReactNode } from "react";
import { ErrorBoundary, type FallbackProps } from "react-error-boundary";

// LoggingErrorBoundary owns the one error-reporting policy the chat surfaces
// share: every caught error is logged with a stable tag and the component stack.
// The visible fallback stays the caller's, because each surface frames a failure
// differently.
interface Props {
  tag: string;
  fallback: ComponentType<FallbackProps>;
  resetKeys?: unknown[];
  children: ReactNode;
}

export function LoggingErrorBoundary({ tag, fallback, resetKeys, children }: Props) {
  return (
    <ErrorBoundary
      FallbackComponent={fallback}
      resetKeys={resetKeys ?? []}
      onError={(error, info) => {
        console.error(`[${tag}]:`, error, info.componentStack);
      }}
    >
      {children}
    </ErrorBoundary>
  );
}

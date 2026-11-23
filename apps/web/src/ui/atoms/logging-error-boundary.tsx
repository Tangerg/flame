import { type ComponentType, type ReactNode } from "react";
import { ErrorBoundary, type FallbackProps } from "react-error-boundary";

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

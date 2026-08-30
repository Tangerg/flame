import { QueryClientProvider } from "@tanstack/react-query";
import { MotionConfig } from "motion/react";
import { queryClient } from "@/lib/queryClient";
import { PluginProvider } from "@/plugins/host/PluginProvider";
import { AppRouter } from "@/router";

// Order matters: QueryClient is widest, PluginProvider sits inside it so plugin components
// can use queries, and AppRouter inside Plugins so routes can render contributions.
// MotionConfig's reducedMotion="user" extends the OS "reduce motion" setting to the JS
// animation half, which the CSS @media rule alone misses.
function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <MotionConfig reducedMotion="user">
        <PluginProvider>
          <AppRouter />
        </PluginProvider>
      </MotionConfig>
    </QueryClientProvider>
  );
}

export default App;

import type { AnyPlugin } from "dougong";
import { QueryClientProvider } from "@tanstack/react-query";
import { MotionConfig } from "motion/react";
import { queryClient } from "@/lib/queryClient";
import { PluginProvider } from "@/plugins/host/PluginProvider";
import { AppRouter } from "@/router";

function App({ plugins }: { plugins: AnyPlugin[] }) {
  return (
    <QueryClientProvider client={queryClient}>
      <MotionConfig reducedMotion="user">
        <PluginProvider plugins={plugins}>
          <AppRouter />
        </PluginProvider>
      </MotionConfig>
    </QueryClientProvider>
  );
}

export default App;

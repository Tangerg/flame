import { type AnyPlugin } from "dougong";
import { ApplicationSurface } from "@/app/ApplicationSurface";
import { PluginProvider } from "@/extensions/host/PluginProvider";
import { AppRouter, type AppNavigation } from "@/router";
function App({ plugins, navigation }: { plugins: AnyPlugin[]; navigation: AppNavigation }) {
  return (
    <ApplicationSurface>
      <PluginProvider plugins={plugins}>
        <AppRouter navigation={navigation} />
      </PluginProvider>
    </ApplicationSurface>
  );
}

export default App;

import { createStore } from "zustand/vanilla";
import { persist, createJSONStorage } from "zustand/middleware";
import { z } from "zod";
import { rehydrateOrDefault } from "@/lib/persistedStore";

const storageKey = "flame.presentation-preferences";
const schema = z.object({ selections: z.record(z.string(), z.string()) });

export function createPresentationPreferences(endpoint: () => string) {
  type State = z.infer<typeof schema>;
  const store = createStore<State>()(
    persist(() => ({ selections: {} }), {
      name: storageKey,
      storage: createJSONStorage(() => localStorage),
      merge: rehydrateOrDefault(storageKey, schema),
    }),
  );
  return {
    endpoint,
    subscribe: store.subscribe,
    read: (key: string) => store.getState().selections[key] ?? null,
    choose(key: string, id: string | null) {
      const selections = { ...store.getState().selections };
      if (id === null) delete selections[key];
      else selections[key] = id;
      store.setState({ selections });
    },
  };
}

import { type SystemAppearance } from "@/features/theme/application/systemAppearance";
export function createSystemAppearance(media: MediaQueryList): SystemAppearance {
  return {
    scheme: () => (media.matches ? "dark" : "light"),
    subscribe(listener) {
      media.addEventListener("change", listener);
      return () => media.removeEventListener("change", listener);
    },
  };
}

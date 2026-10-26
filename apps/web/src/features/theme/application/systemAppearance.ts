import { type Scheme } from "@/lib/appearance";
export interface SystemAppearance {
  scheme(): Scheme;
  subscribe(listener: () => void): () => void;
}

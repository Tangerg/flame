import { startBrowserApplication } from "@/app/browserSupport";
import { showBrowserStartupFailure } from "@/app/browserSupportFailure";
import "@/styles/globals.css";
import "@/styles/startup.css";

void startBrowserApplication(() => import(/* @flame-startup */ "./main")).then(
  (result) => {
    if (result.status === "unsupported") {
      showBrowserStartupFailure(document.getElementById("root")!, result);
    }
  },
  (cause: unknown) => {
    console.error("[desktop] application import failed:", cause);
    showBrowserStartupFailure(document.getElementById("root")!, { status: "failed", cause });
  },
);

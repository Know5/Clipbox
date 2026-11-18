import { useEffect } from "react";
import { EventsOn } from "../wailsjs/runtime/runtime";
import { GetAppSettings } from "./api";
import { refreshAccent, syncWindowBackground } from "./theme";

/** Keeps document theme, window background, and Windows accent color synchronized. */
export function useThemeSync() {
  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: light)");
    let mode = "dark";
    const apply = () => {
      const resolved = mode === "light" || (mode === "system" && media.matches) ? "light" : "dark";
      document.documentElement.setAttribute("data-theme", resolved);
      syncWindowBackground();
      refreshAccent().catch(console.error);
    };
    GetAppSettings()
      .then((settings) => {
        mode = settings?.theme || "dark";
        apply();
      })
      .catch(console.error);
    const unsub = EventsOn("settings:updated", (settings: unknown) => {
      const nextTheme =
        typeof settings === "object" &&
        settings !== null &&
        "theme" in settings &&
        typeof settings.theme === "string"
          ? settings.theme
          : "dark";
      mode = nextTheme;
      apply();
    });
    const onMediaChange = () => {
      if (mode === "system") apply();
    };
    media.addEventListener("change", onMediaChange);
    return () => {
      unsub();
      media.removeEventListener("change", onMediaChange);
    };
  }, []);
}

import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";

const FocusContext = createContext(true);

export function FocusProvider({ children }: { children: ReactNode }) {
  const [windowFocused, setWindowFocused] = useState(document.hasFocus());
  const [visible, setVisible] = useState(document.visibilityState === "visible");

  useEffect(() => {
    const onFocus = () => setWindowFocused(true);
    const onBlur = () => setWindowFocused(false);
    const onVisibility = () => setVisible(document.visibilityState === "visible");
    window.addEventListener("focus", onFocus);
    window.addEventListener("blur", onBlur);
    document.addEventListener("visibilitychange", onVisibility);
    let unlistenFocus: (() => void) | undefined;
    let unlistenVisibility: (() => void) | undefined;
    void import("@tauri-apps/api/window").then(async ({ getCurrentWindow }) => {
      const appWindow = getCurrentWindow();
      unlistenFocus = await appWindow.onFocusChanged(({ payload }) => setWindowFocused(payload));
      unlistenVisibility = await appWindow.onResized(async () => setVisible(await appWindow.isVisible()));
    }).catch(() => undefined);
    return () => {
      window.removeEventListener("focus", onFocus);
      window.removeEventListener("blur", onBlur);
      document.removeEventListener("visibilitychange", onVisibility);
      unlistenFocus?.();
      unlistenVisibility?.();
    };
  }, []);

  return <FocusContext.Provider value={windowFocused && visible}>{children}</FocusContext.Provider>;
}

export const useAppFocused = () => useContext(FocusContext);
export const usePollingInterval = (enabled = true, interval = 10_000) => {
  const focused = useAppFocused();
  return useMemo(() => focused && enabled ? interval : false, [focused, enabled, interval]);
};

import { useEffect, useState } from "react";
import { readRegistry, REGISTRY_CHANGED } from "./registry";

export function useRegistry() {
  const [snapshot, setSnapshot] = useState(readRegistry);
  useEffect(() => {
    const refresh = () => setSnapshot(readRegistry());
    window.addEventListener(REGISTRY_CHANGED, refresh);
    window.addEventListener("storage", refresh);
    return () => {
      window.removeEventListener(REGISTRY_CHANGED, refresh);
      window.removeEventListener("storage", refresh);
    };
  }, []);
  return snapshot;
}

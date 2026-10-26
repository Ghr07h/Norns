import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./app/App";
import { ErrorBoundary } from "./app/error-boundary";
import { Providers } from "./app/providers";
import { i18nReady } from "./i18n";
import "./styles/tokens.css";
import "./styles/global.css";
import "./styles/workspace.css";

await i18nReady;
if (import.meta.env.DEV && import.meta.env.VITE_WELL_DEMO === "1") {
  const { installDemoBridge } = await import("./test/demo-bridge");
  installDemoBridge();
}

createRoot(document.getElementById("root")!).render(<StrictMode><ErrorBoundary><Providers><App /></Providers></ErrorBoundary></StrictMode>);

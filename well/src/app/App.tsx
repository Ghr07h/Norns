import { HashRouter, Navigate, Route, Routes } from "react-router-dom";
import { ServersPage } from "../routes/servers/ServersPage";
import { ServerWorkspace } from "../routes/server/ServerWorkspace";

export function App() {
  return <HashRouter><Routes><Route path="/servers" element={<ServersPage />} /><Route path="/servers/:serverId" element={<ServerWorkspace />} /><Route path="*" element={<Navigate to="/servers" replace />} /></Routes></HashRouter>;
}

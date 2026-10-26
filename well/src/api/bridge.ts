import { invoke } from "@tauri-apps/api/core";

export interface WellBridge {
  invoke<T>(command: string, args?: Record<string, unknown>): Promise<T>;
}

declare global {
  interface Window {
    __WELL_BRIDGE__?: WellBridge;
  }
}

export function bridge(): WellBridge {
  return window.__WELL_BRIDGE__ ?? { invoke };
}

export function invokeCommand<T>(command: string, args?: Record<string, unknown>): Promise<T> {
  return bridge().invoke<T>(command, args);
}

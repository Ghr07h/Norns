import { QueryClient, QueryClientProvider, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { FocusProvider, useAppFocused } from "../polling/focus";

function FocusRefresh() {
  const focused = useAppFocused();
  const client = useQueryClient();
  const previous = useRef(focused);
  useEffect(() => {
    if (focused && !previous.current) void client.refetchQueries({
      type: "active",
      predicate: (query) => query.meta?.serverProbe !== true || query.state.status === "success",
    });
    previous.current = focused;
  }, [client, focused]);
  return null;
}

export function Providers({ children }: { children: ReactNode }) {
  const [client] = useState(() => new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: 2_000, gcTime: 30 * 60_000, refetchOnReconnect: false, refetchOnWindowFocus: false, refetchIntervalInBackground: false },
      mutations: { retry: false },
    },
  }));
  return <QueryClientProvider client={client}><FocusProvider><FocusRefresh />{children}</FocusProvider></QueryClientProvider>;
}

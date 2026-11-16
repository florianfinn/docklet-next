// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager implementation.

import { useState, type ReactNode } from "react";
import { vi } from "vitest";
import { IntlProvider } from "use-intl";
import { MemoryRouter } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { catalogs } from "@/app/i18n/catalog";
import { AdapterContext } from "@/data/use-adapter";
import type { DockletAdapter } from "@/data/adapter";
import { queryKeys } from "@/data/query-keys";

export function Providers({
  children,
  locale = "en-US",
  adapter,
}: {
  children: ReactNode;
  locale?: "de" | "en-US";
  adapter?: DockletAdapter;
}) {
  const [client] = useState(() => {
    const c = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    c.setQueryData(queryKeys.currentUser, {
      id: "example-user",
      username: "example-admin",
      isAdmin: true,
      requiresPasswordChange: false,
    });
    return c;
  });
  const [fallback] = useState(
    () =>
      ({
        kind: "http",
        subscribeChanges: vi.fn(() => () => {}),
        listHosts: vi.fn(async () => [{ id: "host-one", name: "Example host" }]),
        getCurrentUser: vi.fn(),
      }) as unknown as DockletAdapter,
  );
  return (
    <IntlProvider locale={locale} messages={catalogs[locale]}>
      <QueryClientProvider client={client}>
        <AdapterContext.Provider value={adapter ?? fallback}>
          <MemoryRouter>{children}</MemoryRouter>
        </AdapterContext.Provider>
      </QueryClientProvider>
    </IntlProvider>
  );
}

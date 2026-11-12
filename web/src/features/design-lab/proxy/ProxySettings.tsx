// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 1a4a628bcb7569360c8126b781275c14cfc51074
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { Tabs, TabsContent } from "@/shared/ui/tabs";
import { ProxyTabs } from "./ProxyTabs";
import { ConfigurationView } from "./ConfigurationView";
import { ProxySetup } from "./ProxySetup";
import { useProxyText } from "./use-proxy-text";
export function ProxySettings({ instance }: { instance: string }) {
  const t = useProxyText();
  return <Tabs defaultValue="connection"><ProxyTabs options={[{ value: "connection", label: t("connectionSettings") }, { value: "files", label: t("fileSettings") }]} /><TabsContent value="connection"><ProxySetup instance={instance} /></TabsContent><TabsContent value="files"><ConfigurationView instance={instance} /></TabsContent></Tabs>;
}

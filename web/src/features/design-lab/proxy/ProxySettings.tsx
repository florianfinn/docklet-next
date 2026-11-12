import { Tabs, TabsContent } from "@/shared/ui/tabs";
import { ProxyTabs } from "./ProxyTabs";
import { ConfigurationView } from "./ConfigurationView";
import { ProxySetup } from "./ProxySetup";
import { useProxyText } from "./use-proxy-text";
export function ProxySettings({ instance }: { instance: string }) {
  const t = useProxyText();
  return <Tabs defaultValue="connection"><ProxyTabs options={[{ value: "connection", label: t("connectionSettings") }, { value: "files", label: t("fileSettings") }]} /><TabsContent value="connection"><ProxySetup instance={instance} /></TabsContent><TabsContent value="files"><ConfigurationView instance={instance} /></TabsContent></Tabs>;
}

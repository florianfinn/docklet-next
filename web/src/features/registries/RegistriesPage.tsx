// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230.

import { Package } from "lucide-react";
import { useState } from "react";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import type { Registry } from "@/domain/registries";
import { useImageError } from "@/features/images/use-image-error";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { captureCommandFailure } from "@/shared/components/command-failure";
import { EmptyState, PageHeader, QueryError } from "@/shared/components/page";
import { Button } from "@/shared/ui/button";
import { Card } from "@/shared/ui/card";
import { RegistryDialog } from "./RegistryDialog";
import { RegistryTestDialog } from "./RegistryTestDialog";
import { useRegistries } from "./use-registries";

function RegistryRow({ registry, onEdit, onTest, onDelete }: { registry: Registry; onEdit: () => void; onTest: () => void; onDelete: () => void }) {
  const t = useTranslations("registries"), ref = { kind: "registry", id: registry.id };
  const edit = useCapability("updateContainerRegistry", ref), test = useCapability("testContainerRegistry", ref), remove = useCapability("deleteContainerRegistry", ref);
  return <Card className="flex flex-col gap-3 p-4"><div className="min-w-0"><h2 className="break-all font-semibold">{registry.url}</h2><p className="text-sm text-muted-foreground">{registry.description}</p><p className="text-sm">{t(registry.hasCredential ? "stored" : "noCredential")}</p><p className="text-xs text-muted-foreground">{t("type")}: {registry.registryType === "generic" ? t("generic") : registry.registryType === "ecr" ? t("ecr") : t("unsupported")} · {t(registry.enabled ? "enabled" : "disabled")}</p></div><div className="flex flex-wrap gap-2"><Button variant="outline" disabled={!edit.available} title={edit.reason} onClick={onEdit}>{t("edit")}</Button><Button variant="outline" disabled={!test.available} title={test.reason} onClick={onTest}>{t("test")}</Button><Button variant="outline" disabled={!remove.available} title={remove.reason} onClick={onDelete}>{t("remove")}</Button></div></Card>;
}
export default function RegistriesPage() {
  const t = useTranslations("registries"), rows = useRegistries(), adapter = useAdapter(), error = useImageError();
  const create = useCapability("createContainerRegistry");
  const [editing, setEditing] = useState<Registry | "new" | null>(null), [testing, setTesting] = useState<Registry | null>(null), [removing, setRemoving] = useState<Registry | null>(null);
  return <>
    <PageHeader title={t("title")} description={t("description")} actions={<Button disabled={!create.available} title={create.reason} onClick={() => setEditing("new")}>{t("create")}</Button>} />
    {!rows.capability.available ? <p role="status">{rows.capability.reason}</p> : rows.isError ? <QueryError error={rows.error} onRetry={() => void rows.refetch()} /> : rows.isPending ? <p aria-busy="true">{t("title")}</p> : !rows.data?.length ? <EmptyState icon={<Package />} title={t("empty")} /> : <div className="grid gap-4 lg:grid-cols-2">{rows.data.map((registry) => <RegistryRow key={registry.id} registry={registry} onEdit={() => setEditing(registry)} onTest={() => setTesting(registry)} onDelete={() => setRemoving(registry)} />)}</div>}
    {editing && <RegistryDialog key={editing === "new" ? "new" : editing.id} registry={editing === "new" ? undefined : editing} onClose={() => setEditing(null)} />}
    {testing && <RegistryTestDialog key={testing.id} registry={testing} onClose={() => setTesting(null)} />}
    <ConfirmDialog open={!!removing} onOpenChange={(open) => { if (!open) setRemoving(null); }} title={t("deleteTitle")} description={t("deleteBody", { url: removing?.url ?? "" })} confirmLabel={t("remove")} destructive onConfirm={async () => {
      if (!removing) return false;
      const failure = captureCommandFailure(adapter);
      try { await adapter.deleteContainerRegistry(removing.id); return true; }
      catch (cause) { failure.error(error(cause)); return false; }
    }} />
  </>;
}

// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230.

import { useState } from "react";
import { toast } from "sonner";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import { registryHost, type Registry, type RegistryDraft } from "@/domain/registries";
import { useImageError } from "@/features/images/use-image-error";
import { TextField } from "@/shared/components/form-field";
import { DialogCommandFooter } from "@/shared/components/dialog-command";
import { useDialogCommand } from "@/shared/components/use-dialog-command";
import { BusyButton } from "@/shared/components/busy-button";
import { useBusyAction } from "@/shared/components/use-busy-action";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { Label } from "@/shared/ui/label";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Textarea } from "@/shared/ui/textarea";

/** Mounted per editing identity; only fresh write-only credential inputs enter this form. */
export function RegistryDialog({ registry, onClose }: { registry?: Registry; onClose: () => void }) {
  const t = useTranslations("registries"), adapter = useAdapter(), error = useImageError();
  const operation = registry ? "updateContainerRegistry" : "createContainerRegistry";
  const capability = useCapability(operation, registry ? { kind: "registry", id: registry.id } : undefined);
  const clear = useCapability("clearContainerRegistryCredential", { kind: "registry", id: registry?.id ?? "" });
  const feedback = useDialogCommand(true, registry?.id, error);
  const [draft, setDraft] = useState<RegistryDraft>(() => ({ url: registry?.url ?? "", description: registry?.description ?? "", enabled: registry?.enabled ?? true, insecure: registry?.insecure ?? false,
    registryType: registry?.registryType === "ecr" ? "ecr" : "generic", awsRegion: registry?.awsRegion ?? "", repositoryNames: registry?.repositoryNames ?? [] }));
  const patch = <K extends keyof RegistryDraft>(key: K, value: RegistryDraft[K]) => setDraft((old) => ({ ...old, [key]: value }));
  const [repositories, setRepositories] = useState(registry?.repositoryNames.join("\n") ?? "");
  const credentialsMissing = !registry && draft.registryType === "generic" && (!draft.username?.trim() || !draft.token?.trim());
  const valid = !credentialsMissing && !!registryHost(draft.url) && (!registry || ["generic", "ecr"].includes(registry.registryType));
  const save = useBusyAction(async () => {
    if (!valid || !capability.available) return;
    const input = { ...draft, repositoryNames: repositories.split("\n").map((line) => line.trim()).filter(Boolean) };
    const result = await feedback.run(() => registry ? adapter.updateContainerRegistry(registry.id, input) : adapter.createContainerRegistry(input), (result) => { if (result) toast.success(t("saved", { url: input.url })); });
    if (result) onClose();
  });
  const checkbox = (key: "enabled" | "insecure" | "clearCredential", label: string) => <div className="flex items-center gap-2"><Checkbox id={`registry-${key}`} disabled={key === "clearCredential" && !clear.available} checked={!!draft[key]} onCheckedChange={(value) => patch(key, value === true)} /><Label htmlFor={`registry-${key}`}>{label}</Label></div>;
  return <Dialog open onOpenChange={(open) => { if (!open) feedback.change(false, onClose); }}>
    <DialogContent {...feedback.bindings} className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-lg">
      <DialogHeader><DialogTitle>{t(registry ? "editTitle" : "createTitle")}</DialogTitle><DialogDescription>{t("credentialsHint")}</DialogDescription></DialogHeader>
      <form id="registry-form" className="flex flex-col gap-4" onSubmit={(event) => { event.preventDefault(); void save.run(); }} autoComplete="off">
        <fieldset disabled={save.busy} className="contents">
        <TextField id="registry-url" label={t("url")} value={draft.url} onChange={(value) => patch("url", value)} error={draft.url && !registryHost(draft.url) ? t("invalid") : undefined} />
        <TextField id="registry-description" label={t("descriptionLabel")} value={draft.description} onChange={(value) => patch("description", value)} />
        <div className="flex flex-col gap-2"><Label htmlFor="registry-type">{t("type")}</Label><Select value={draft.registryType} onValueChange={(value) => { if (value === "generic" || value === "ecr") setDraft((old) => ({ ...old, registryType: value, username: undefined, token: undefined, awsAccessKeyId: undefined, awsSecretAccessKey: undefined })); }}><SelectTrigger id="registry-type"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="generic">{t("generic")}</SelectItem><SelectItem value="ecr">{t("ecr")}</SelectItem></SelectContent></Select></div>
        {credentialsMissing && <p className="text-sm">{t("genericRequired")}</p>}
        {registry && <p className="text-sm">{t(registry.hasCredential ? "stored" : "noCredential")}</p>}
        {checkbox("enabled", t("enabled"))}{checkbox("insecure", t("insecure"))}
        {registry && checkbox("clearCredential", t("clear"))}
        {draft.registryType === "ecr" && <TextField id="registry-region" label={t("region")} value={draft.awsRegion ?? ""} onChange={(value) => patch("awsRegion", value)} />}
        {!draft.clearCredential && <>
          {draft.registryType === "generic" ? <>
            <TextField id="registry-username" label={t("username")} type="password" autoComplete="new-password" value={draft.username ?? ""} onChange={(value) => patch("username", value)} />
            <TextField id="registry-token" label={t("valueLabel")} type="password" autoComplete="new-password" value={draft.token ?? ""} onChange={(value) => patch("token", value)} />
          </> : <>
            <TextField id="registry-access" label={t("keyIdentityLabel")} type="password" autoComplete="new-password" value={draft.awsAccessKeyId ?? ""} onChange={(value) => patch("awsAccessKeyId", value)} />
            <TextField id="registry-secret" label={t("keyValueLabel")} type="password" autoComplete="new-password" value={draft.awsSecretAccessKey ?? ""} onChange={(value) => patch("awsSecretAccessKey", value)} />
          </>}
        </>}
        <div className="flex flex-col gap-2"><Label htmlFor="registry-repositories">{t("repositories")}</Label><Textarea id="registry-repositories" value={repositories} onChange={(event) => setRepositories(event.target.value)} /></div>
        {!valid && registry && !["generic", "ecr"].includes(registry.registryType) && <p role="alert">{t("unsupported")}</p>}
        </fieldset>
      </form>
      <DialogCommandFooter feedback={feedback}><Button variant="outline" onClick={() => feedback.change(false, onClose)}>{t("cancel")}</Button><BusyButton type="submit" form="registry-form" disabled={!valid || !capability.available} title={capability.reason} busy={save.busy}>{t("save")}</BusyButton></DialogCommandFooter>
    </DialogContent>
  </Dialog>;
}

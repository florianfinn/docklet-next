import { Plug, TriangleAlert } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { TextField } from "@/shared/components/form-field";
import { Alert, AlertDescription } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Label } from "@/shared/ui/label";
import { Spinner } from "@/shared/ui/spinner";

import { layoutStore, setService, useLayout, type ServiceKind } from "../dashboard-layout";
import { AppIcon } from "./app-icons";
import { connectingStore, simulateConnect, SERVICE_META } from "./service-data";

const CONNECT_MS = 900;

function ConfigForm({ kind, onDone }: { kind: ServiceKind; onDone: () => void }) {
  const t = useTranslations("dashboard.services");
  const config = useLayout().services[kind];
  const [address, setAddress] = useState(config.address);
  // Only a placeholder field: the value stays in this form and is dropped on close.
  const [token, setToken] = useState("");
  const [fail, setFail] = useState(config.simulateError);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<"error" | "unconfigured" | null>(null);

  const connect = () => {
    setBusy(true);
    setResult(null);
    connectingStore.set((s) => [...new Set([...s, kind])]);
    setTimeout(() => {
      const status = simulateConnect(address, fail);
      layoutStore.set((l) => setService(l, kind, { address: address.trim(), status, simulateError: fail }));
      connectingStore.set((s) => s.filter((k) => k !== kind));
      setBusy(false);
      if (status === "connected") {
        toast.success(t("connected", { name: t(`${kind}.name`) }));
        onDone();
      } else setResult(status);
    }, CONNECT_MS);
  };

  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault();
        connect();
      }}
    >
      <TextField
        id={`svc-${kind}-address`}
        label={t("address")}
        value={address}
        onChange={setAddress}
        placeholder={SERVICE_META[kind].exampleAddress}
        description={t("addressHint")}
        mono
        trailing={
          <Button type="button" variant="outline" size="sm" onClick={() => setAddress(SERVICE_META[kind].exampleAddress)}>
            {t("example")}
          </Button>
        }
      />
      <TextField id={`svc-${kind}-token`} label={t(`${kind}.secret`)} type="password" value={token} onChange={setToken} placeholder={t("secretPlaceholder")} description={t("secretHint")} autoComplete="off" />
      <div className="flex items-center gap-2">
        <Checkbox id={`svc-${kind}-fail`} checked={fail} onCheckedChange={(v) => setFail(v === true)} />
        <Label htmlFor={`svc-${kind}-fail`} className="text-sm font-normal">
          {t("simulateError")}
        </Label>
      </div>
      {result && (
        <Alert variant="destructive" className="border-destructive/30 bg-destructive/5">
          <TriangleAlert />
          <AlertDescription className="text-xs">{result === "error" ? t("errorBody", { address }) : t("missingBody")}</AlertDescription>
        </Alert>
      )}
      <p className="text-[11px] text-muted-foreground">{t("demoNote")}</p>
      <DialogFooter>
        <Button type="submit" disabled={busy}>
          {busy ? <Spinner data-icon="inline-start" /> : <Plug data-icon="inline-start" />}
          {busy ? t("connecting") : t("connect")}
        </Button>
      </DialogFooter>
    </form>
  );
}

/** Address and placeholder credentials of a service widget with a simulated connect. */
export function ServiceConfigDialog({ kind, onOpenChange }: { kind: ServiceKind | null; onOpenChange: (open: boolean) => void }) {
  const t = useTranslations("dashboard.services");
  return (
    <Dialog open={!!kind} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        {kind && (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2.5">
                <AppIcon kind={kind} />
                {t("configTitle", { name: t(`${kind}.name`) })}
              </DialogTitle>
              <DialogDescription>{t(`${kind}.desc`)}</DialogDescription>
            </DialogHeader>
            <ConfigForm key={kind} kind={kind} onDone={() => onOpenChange(false)} />
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}

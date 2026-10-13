import { FlaskConical } from "lucide-react";
import { useState } from "react";
import { useTranslations } from "use-intl";

import type { ContainerInfo, PortMapping } from "@/domain/model";
import { imageRef } from "@/domain/updates";
import { useContainerCommands } from "@/shared/hooks/use-container-commands";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { HelpLabel, HelpTarget } from "@/shared/help/help-tooltip";
import { Field, FieldDescription, FieldGroup } from "@/shared/ui/field";
import { Input } from "@/shared/ui/input";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Textarea } from "@/shared/ui/textarea";

function parsePorts(text: string): PortMapping[] {
  return text
    .split("\n")
    .map((l) => l.trim())
    .filter(Boolean)
    .map((l) => {
      const [host, container = ""] = l.includes(":") ? l.split(":") : ["", l];
      return { host: host || undefined, container };
    });
}

function parseEnv(text: string): Record<string, string> {
  return Object.fromEntries(
    text
      .split("\n")
      .map((l) => l.trim())
      .filter((l) => l && l.includes("="))
      .map((l) => [l.slice(0, l.indexOf("=")), l.slice(l.indexOf("=") + 1)]),
  );
}

/** Editor for standalone containers; Unraid and unclear origins never reach it. */
export function RecreateDialog({ container, open, onOpenChange }: { container: ContainerInfo | undefined; open: boolean; onOpenChange: (o: boolean) => void }) {
  const t = useTranslations("containers.recreate");
  const tc = useTranslations("common.actions");
  const cmd = useContainerCommands();
  const [image, setImage] = useState("");
  const [restart, setRestart] = useState("unless-stopped");
  const [ports, setPorts] = useState("");
  const [env, setEnv] = useState("");
  const [seen, setSeen] = useState<string | null>(null);
  if (container && seen !== container.id) {
    setSeen(container.id);
    setImage(imageRef(container.image));
    setRestart(container.restartPolicy || "no");
    setPorts(container.ports.map((p) => (p.host ? `${p.host}:${p.container}` : p.container)).join("\n"));
    setEnv(Object.entries(container.env).map(([k, v]) => `${k}=${v}`).join("\n"));
  }
  if (!container) return null;
  const submit = (fail?: boolean) => {
    void cmd.recreate(container.id, { image, restartPolicy: restart, ports: parsePorts(ports), env: parseEnv(env) }, fail ? "start" : undefined);
    onOpenChange(false);
  };
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("title", { name: container.name })}</DialogTitle>
          <DialogDescription>{t("description")}</DialogDescription>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <HelpLabel htmlFor="rc-image" topic="service.image">
              {t("image")}
            </HelpLabel>
            <Input id="rc-image" className="font-mono" value={image} onChange={(e) => setImage(e.target.value)} />
          </Field>
          <Field>
            <HelpLabel htmlFor="rc-restart" topic="service.restart">
              {t("restart")}
            </HelpLabel>
            <Select value={restart} onValueChange={setRestart}>
              <SelectTrigger id="rc-restart" className="w-full font-mono">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {["no", "always", "on-failure", "unless-stopped"].map((r) => (
                    <SelectItem key={r} value={r} className="font-mono">
                      {r}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>
          <Field>
            <HelpLabel htmlFor="rc-ports" topic="container.ports">
              {t("ports")}
            </HelpLabel>
            <Textarea id="rc-ports" rows={2} className="font-mono text-xs" value={ports} onChange={(e) => setPorts(e.target.value)} />
            <FieldDescription>{t("portsHint")}</FieldDescription>
          </Field>
          <Field>
            <HelpLabel htmlFor="rc-env" topic="container.env">
              {t("env")}
            </HelpLabel>
            <Textarea id="rc-env" rows={3} className="font-mono text-xs" value={env} onChange={(e) => setEnv(e.target.value)} />
            <FieldDescription>{t("envHint")}</FieldDescription>
          </Field>
        </FieldGroup>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {tc("cancel")}
          </Button>
          {import.meta.env.DEV && <Button variant="outline" onClick={() => submit(true)}>
            <FlaskConical data-icon="inline-start" /> {t("failDemo")}
          </Button>}
          <HelpTarget topic="container.recreate">
            <Button disabled={!image.trim()} onClick={() => submit()}>
              {t("submit")}
            </Button>
          </HelpTarget>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

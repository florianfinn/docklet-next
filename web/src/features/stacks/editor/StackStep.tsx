import { isHostAvailable } from "@/domain/hosts";
import { ArrowRight, Layers, Lock } from "lucide-react";
import { useTranslations } from "use-intl";

import type { ComposeModel } from "@/domain/compose/model";
import { ops } from "@/domain/compose/ops";
import type { Host } from "@/domain/model";
import { IconTile } from "@/shared/components/tone";
import { Badge } from "@/shared/ui/badge";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from "@/shared/ui/card";
import { HelpLabel } from "@/shared/help/help-tooltip";
import { Field, FieldDescription, FieldError, FieldGroup } from "@/shared/ui/field";
import { Input } from "@/shared/ui/input";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Textarea } from "@/shared/ui/textarea";

import { useEditorText } from "./editor-text";
import { LabelEditor } from "./fields";
import type { ComposeDraft } from "./use-compose-draft";

interface StackStepProps {
  draft: ComposeDraft;
  model: ComposeModel;
  hosts: Host[];
  isNew: boolean;
  labelSuggestions: string[];
  onEditError: (message: string) => void;
  onNext: () => void;
}

export function StackStep({ draft, model, hosts, isNew, labelSuggestions, onEditError, onNext }: StackStepProps) {
  const t = useTranslations("editor.stack");
  const ts = useTranslations("editor.steps");
  const text = useEditorText();
  const nameIssue = draft.issues.find((i) => i.field === "name");
  const host = hosts.find((h) => h.id === draft.hostId);
  const projectDir = `${host?.projectDir ?? "/opt/stacks"}/${model.name || "<stackname>"}`;
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2.5">
          <IconTile tone="accent" size="sm">
            <Layers />
          </IconTile>
          {t("title")}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <FieldGroup className="gap-5">
          <div className="grid gap-5 md:grid-cols-2">
            <Field data-invalid={nameIssue?.level === "error" || undefined}>
              <HelpLabel htmlFor="ed-name" topic="stack.name">
                {t("name")} <span className="text-destructive">*</span>
              </HelpLabel>
              <Input
                id="ed-name"
                className="font-mono"
                required
                aria-invalid={nameIssue?.level === "error" || undefined}
                value={model.name}
                placeholder="media"
                onChange={(e) => {
                  const err = draft.edit(ops.setName(e.target.value));
                  if (err) onEditError(text.editError(err));
                }}
              />
              {nameIssue ? <FieldError>{text.issue(nameIssue)}</FieldError> : <FieldDescription>{t("nameHint")}</FieldDescription>}
            </Field>
            <Field>
              <HelpLabel htmlFor="ed-host" topic="stack.host">
                {t("host")}
              </HelpLabel>
              <Select value={draft.hostId} onValueChange={draft.setHostId} disabled={!isNew}>
                <SelectTrigger id="ed-host" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {hosts.map((h) => (
                      <SelectItem key={h.id} value={h.id} disabled={!isHostAvailable(h)}>
                        <i className="size-2.5 rounded-[3px]" style={{ background: h.color }} aria-hidden />
                        {!isHostAvailable(h) ? t("hostOffline", { name: h.name }) : h.name}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
              <FieldDescription>{isNew ? t("hostHint") : t("hostFixed")}</FieldDescription>
            </Field>
          </div>
          <Field>
            <HelpLabel topic="stack.labels">{t("labels")}</HelpLabel>
            <LabelEditor
              labels={draft.labels}
              onChange={draft.setLabels}
              suggestions={labelSuggestions}
              placeholder={t("labelPlaceholder")}
              addLabel={t("labelAdd")}
              removeLabel={(label) => t("labelRemove", { label })}
              emptyText={t("labelsNone")}
            />
            <FieldDescription>{t("labelsHint")}</FieldDescription>
          </Field>
          <Field>
            <HelpLabel htmlFor="ed-dir" topic="stack.projectDir">
              {t("projectDir")}
            </HelpLabel>
            <Input id="ed-dir" readOnly value={projectDir} className="font-mono text-muted-foreground" />
            <FieldDescription>{t("projectDirHint")}</FieldDescription>
          </Field>
          <Field>
            <HelpLabel htmlFor="ed-env" topic="stack.env">
              {t("env")}
            </HelpLabel>
            <Textarea id="ed-env" rows={3} spellCheck={false} className="font-mono text-xs" value={draft.env} onChange={(e) => draft.setEnv(e.target.value)} />
            <FieldDescription>{t("envHint")}</FieldDescription>
          </Field>
          {model.extras.length > 0 && (
            <div className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
              <Lock className="size-3.5" aria-hidden />
              {t("extras")}
              {model.extras.map((x) => (
                <Badge key={x} variant="outline" className="font-mono">
                  {x}
                </Badge>
              ))}
            </div>
          )}
        </FieldGroup>
      </CardContent>
      <CardFooter className="justify-end border-t">
        <Button size="sm" onClick={onNext} className="group">
          {ts("next")}: {ts("services")}
          <ArrowRight data-icon="inline-end" className="nudge" />
        </Button>
      </CardFooter>
    </Card>
  );
}

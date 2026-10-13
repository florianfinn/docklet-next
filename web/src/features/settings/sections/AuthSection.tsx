import { Pencil, Plus, ShieldCheck, Trash2, UserRound } from "lucide-react";
import { useState } from "react";
import { useTranslations } from "use-intl";

import { useStacks } from "@/data/queries";
import type { AccessPreset, HubSettings, HubUser } from "@/domain/settings";
import { email, required } from "@/domain/validation";
import { ChoiceCards, TextField } from "@/shared/components/form-field";
import { ToneBadge } from "@/shared/components/tone";
import { draftId } from "@/shared/lib/ids";
import { cn } from "@/shared/lib/utils";
import { Alert, AlertDescription } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { HelpLegend } from "@/shared/help/help-tooltip";
import { FieldLegend, FieldSet } from "@/shared/ui/field";
import { Label } from "@/shared/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui/table";

import { useAuthErrors, type PasswordDraft } from "../field-rules";
import { SettingsForm } from "../SettingsForm";
import { useSaveSettings } from "../use-save-settings";
import { useSectionDraft } from "../use-section-draft";
import { AuthModeFields } from "./AuthModeFields";

function UserDialog({ user, onClose, onApply }: { user: HubUser | null; onClose: () => void; onApply: (u: HubUser) => void }) {
  const t = useTranslations("settings.users");
  const tv = useTranslations("hosts.validation");
  const stacks = useStacks();
  const [draft, setDraft] = useState<HubUser>(() => user ?? { id: draftId("u"), name: "", email: "", access: "operate", scope: "all" });
  const [tried, setTried] = useState(false);
  const nameErr = required(draft.name);
  const mailErr = email(draft.email);
  const scope = draft.scope === "all" ? [] : draft.scope;
  const toggle = (id: string) => {
    const next = scope.includes(id) ? scope.filter((x) => x !== id) : [...scope, id];
    setDraft({ ...draft, scope: next.length ? next : "all" });
  };
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <form
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            setTried(true);
            if (nameErr || mailErr) return;
            onApply(draft);
          }}
        >
          <DialogHeader>
            <DialogTitle>{user ? t("dialogEdit", { name: user.name }) : t("dialogAdd")}</DialogTitle>
            <DialogDescription>{t("description")}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 sm:grid-cols-2">
            <TextField id="usr-name" label={t("name")} value={draft.name} onChange={(name) => setDraft({ ...draft, name })} error={tried && nameErr ? tv(nameErr as "required") : undefined} />
            <TextField id="usr-mail" label={t("email")} type="email" value={draft.email} onChange={(m) => setDraft({ ...draft, email: m })} error={tried && mailErr ? (mailErr === "required" ? tv("required") : t("emailInvalid")) : undefined} placeholder="name@example.com" />
          </div>
          <FieldSet>
            <HelpLegend topic="settingsAuth.access">{t("access")}</HelpLegend>
            <ChoiceCards<AccessPreset>
              label={t("access")}
              value={draft.access}
              onChange={(access) => setDraft({ ...draft, access })}
              options={[
                { value: "full", title: t("full"), description: t("fullDesc") },
                { value: "operate", title: t("operate"), description: t("operateDesc") },
              ]}
            />
          </FieldSet>
          <FieldSet className="gap-2">
            <FieldLegend variant="label" className="mb-1">
              {t("stacks")}
            </FieldLegend>
            <p className="text-xs text-muted-foreground">{t("stacksHint")}</p>
            <div className="grid max-h-40 grid-cols-2 gap-1.5 overflow-auto sm:grid-cols-3">
              {(stacks.data ?? []).map((s) => (
                <Label key={s.id} className="flex items-center gap-2 rounded-lg border px-2 py-1.5 text-sm font-normal">
                  <Checkbox checked={scope.includes(s.id)} onCheckedChange={() => toggle(s.id)} />
                  {s.name}
                </Label>
              ))}
            </div>
          </FieldSet>
          {draft.access === "full" && <p className="text-xs text-warning">{t("limitNote")}</p>}
          <DialogFooter>
            <Button type="submit">{t("apply")}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export function AuthSection({ settings }: { settings: HubSettings }) {
  const t = useTranslations("settings");
  const tu = useTranslations("settings.users");
  const save = useSaveSettings();
  const section = useSectionDraft({ auth: settings.auth, users: settings.users });
  const [pw, setPw] = useState<PasswordDraft>({ password: "", confirm: "" });
  const [editing, setEditing] = useState<HubUser | "new" | null>(null);
  const draft = section.draft ?? { auth: settings.auth, users: settings.users };
  const errors = useAuthErrors(draft.auth, pw);
  const dirty = section.dirty || pw.password.length > 0;
  const admins = draft.users.filter((u) => u.admin && u.access === "full").length;

  return (
    <SettingsForm
      title={t("sections.auth")}
      dirty={dirty}
      valid={Object.keys(errors).length === 0}
      onDiscard={() => {
        section.reset();
        setPw({ password: "", confirm: "" });
      }}
      onSave={async () => {
        const auth = { ...draft.auth, local: { ...draft.auth.local, passwordSet: draft.auth.local.passwordSet || pw.password.length > 0 } };
        await save("auth", auth);
        await save("users", draft.users);
        section.commit({ auth, users: draft.users }, draft);
        // Edits kept from during the save still need the stored password flag.
        section.setDraft((d) => (d && d.auth.local.passwordSet !== auth.local.passwordSet ? { ...d, auth: { ...d.auth, local: { ...d.auth.local, passwordSet: auth.local.passwordSet } } } : d));
        // The password is dropped (only "passwordSet" is kept) unless edited during the save.
        setPw((cur) => (cur.password === pw.password && cur.confirm === pw.confirm ? { password: "", confirm: "" } : cur));
      }}
    >
      <AuthModeFields value={draft.auth} onChange={(auth) => section.patch({ auth })} password={pw} onPassword={setPw} showErrors={dirty} />
      <section aria-labelledby="users-title" className="flex flex-col gap-3 border-t pt-5">
        <div className="flex flex-wrap items-end gap-2">
          <div className="mr-auto">
            <h3 id="users-title" className="text-sm font-semibold">
              {tu("title")}
            </h3>
            <p className="text-xs text-muted-foreground">{tu("description")}</p>
          </div>
          <Button type="button" size="sm" variant="outline" disabled={draft.auth.mode === "none"} onClick={() => setEditing("new")}>
            <Plus data-icon="inline-start" />
            {tu("add")}
          </Button>
        </div>
        {draft.auth.mode === "none" ? (
          <p className="rounded-xl border border-dashed p-4 text-sm text-muted-foreground">{tu("disabledNone")}</p>
        ) : (
          <>
            {draft.auth.mode === "proxy" && <p className="text-xs text-muted-foreground">{tu("proxySource")}</p>}
            <div className="overflow-x-auto rounded-xl border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{tu("name")}</TableHead>
                    <TableHead>{tu("access")}</TableHead>
                    <TableHead>{tu("scope")}</TableHead>
                    <TableHead className="text-right">
                      <span className="sr-only">{tu("edit")}</span>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {draft.users.map((u) => {
                    const lastAdmin = u.admin && admins <= 1;
                    return (
                      <TableRow key={u.id}>
                        <TableCell>
                          <span className="flex items-center gap-2 font-medium">
                            <UserRound className="size-3.5 text-muted-foreground" aria-hidden />
                            {u.name}
                            {u.admin && (
                              <ToneBadge tone="violet" className="h-5 px-1.5 text-[10.5px]">
                                <ShieldCheck aria-hidden />
                                {tu("admin")}
                              </ToneBadge>
                            )}
                          </span>
                          <span className="text-xs text-muted-foreground">{u.email}</span>
                        </TableCell>
                        <TableCell>
                          <ToneBadge tone={u.access === "full" ? "accent" : "sky"}>{u.access === "full" ? tu("full") : tu("operate")}</ToneBadge>
                        </TableCell>
                        <TableCell className="text-xs">{u.scope === "all" ? tu("scopeAll") : tu("scopeSome", { count: u.scope.length })}</TableCell>
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-1">
                            <Button type="button" size="icon" variant="ghost" className="size-8" aria-label={`${tu("edit")}: ${u.name}`} disabled={u.admin} onClick={() => setEditing(u)}>
                              <Pencil />
                            </Button>
                            <Button
                              type="button"
                              size="icon"
                              variant="ghost"
                              className={cn("size-8 text-destructive hover:text-destructive")}
                              aria-label={`${tu("remove")}: ${u.name}`}
                              title={lastAdmin ? tu("lastAdmin") : undefined}
                              disabled={lastAdmin}
                              onClick={() => section.patch({ users: draft.users.filter((x) => x.id !== u.id) })}
                            >
                              <Trash2 />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </div>
            <Alert className="border-warning/30 bg-warning/6">
              <ShieldCheck className="text-warning" />
              <AlertDescription className="text-xs">{tu("limitNote")}</AlertDescription>
            </Alert>
          </>
        )}
      </section>
      {editing && (
        <UserDialog
          user={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
          onApply={(u) => {
            const exists = draft.users.some((x) => x.id === u.id);
            section.patch({ users: exists ? draft.users.map((x) => (x.id === u.id ? u : x)) : [...draft.users, u] });
            setEditing(null);
          }}
        />
      )}
    </SettingsForm>
  );
}

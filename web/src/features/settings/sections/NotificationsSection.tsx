import { useCapability } from "@/data/use-capability";
import { ArrowLeft, BellRing, Link2, Mail, MessageCircle, Pencil, Plus, Send, Trash2, Webhook } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { useAdapter } from "@/data/use-adapter";
import { ActionPresentationPointer } from "@/features/activity/ActionPresentationSetting";
import { CHANNEL_KINDS, notificationBaseUrl, NOTIFICATION_EVENTS, type ChannelKind, type HubSettings, type NotificationChannel, type NotificationEvent } from "@/domain/settings";
import { required } from "@/domain/validation";
import { TextField } from "@/shared/components/form-field";
import { ToneBadge } from "@/shared/components/tone";
import { useAdapterText } from "@/data/use-adapter-text";
import { draftId } from "@/shared/lib/ids";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { HelpLegend, HelpTarget } from "@/shared/help/help-tooltip";
import { FieldSet } from "@/shared/ui/field";
import { Label } from "@/shared/ui/label";
import { Spinner } from "@/shared/ui/spinner";
import { Switch } from "@/shared/ui/switch";

import { SettingsForm } from "../SettingsForm";
import { useSaveSettings } from "../use-save-settings";
import { useSectionDraft } from "../use-section-draft";

function KindIcon({ kind }: { kind: ChannelKind }) {
  if (kind === "email") return <Mail />;
  if (kind === "generic") return <Webhook />;
  if (kind === "ntfy" || kind === "pushover" || kind === "gotify") return <BellRing />;
  return <MessageCircle />;
}

function ChannelDialog({ channel, onClose, onApply }: { channel: NotificationChannel | null; onClose: () => void; onApply: (c: NotificationChannel) => void }) {
  const t = useTranslations("settings.notifications");
  const tv = useTranslations("hosts.validation");
  const [kind, setKind] = useState<ChannelKind | null>(channel?.kind ?? null);
  const [draft, setDraft] = useState<NotificationChannel | null>(channel);
  const [secret, setSecret] = useState("");
  const [tried, setTried] = useState(false);
  const choose = (k: ChannelKind) => {
    setKind(k);
    setDraft({ id: draftId("n"), kind: k, name: t(`kinds.${k}`), target: "", secretSet: false, enabled: true, events: ["deployFailed", "hostOffline", "backupFailed"] });
  };
  const toggleEvent = (e: NotificationEvent) => draft && setDraft({ ...draft, events: draft.events.includes(e) ? draft.events.filter((x) => x !== e) : [...draft.events, e] });
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-xl">
        {!kind || !draft ? (
          <>
            <DialogHeader>
              <DialogTitle>{t("chooseKind")}</DialogTitle>
              <DialogDescription>{t("onlyConfigured")}</DialogDescription>
            </DialogHeader>
            <ul className="grid grid-cols-2 gap-2 sm:grid-cols-3">
              {CHANNEL_KINDS.map((k) => (
                <li key={k}>
                  <button type="button" onClick={() => choose(k)} className="lift flex w-full items-center gap-2.5 rounded-xl border bg-card/50 p-3 text-left text-sm outline-none hover:border-primary/50 focus-visible:ring-[3px] focus-visible:ring-ring/50">
                    <span className="text-primary-text [&_svg]:size-4" aria-hidden>
                      <KindIcon kind={k} />
                    </span>
                    {t(`kinds.${k}`)}
                  </button>
                </li>
              ))}
            </ul>
          </>
        ) : (
          <form
            className="flex flex-col gap-4"
            onSubmit={(e) => {
              e.preventDefault();
              setTried(true);
              if (required(draft.name) || required(draft.target)) return;
              onApply({ ...draft, secretSet: draft.secretSet || secret.length > 0 });
            }}
          >
            <DialogHeader>
              <DialogTitle>{channel ? t("dialogEdit", { name: channel.name }) : t("dialogAdd", { kind: t(`kinds.${kind}`) })}</DialogTitle>
              <DialogDescription>{t("secretDemo")}</DialogDescription>
            </DialogHeader>
            <div className="grid gap-4 sm:grid-cols-2">
              <TextField id="ch-name" label={t("name")} value={draft.name} onChange={(name) => setDraft({ ...draft, name })} error={tried && required(draft.name) ? tv("required") : undefined} />
              <TextField id="ch-target" label={t(`targets.${kind}`)} value={draft.target} onChange={(target) => setDraft({ ...draft, target })} error={tried && required(draft.target) ? tv("required") : undefined} mono placeholder={kind === "email" ? "admin@example.com" : kind === "ntfy" || kind === "gotify" || kind === "generic" ? "https://notify.example.com/…" : ""} />
            </div>
            {kind !== "email" && kind !== "ntfy" && <TextField id="ch-secret" type="password" label={t("secret")} help="settingsNotifications.secret" value={secret} onChange={setSecret} description={draft.secretSet ? t("secretKeep") : t("secretDemo")} autoComplete="new-password" />}
            <FieldSet className="gap-2">
              <HelpLegend topic="settingsNotifications.events" className="mb-1">
                {t("events")}
              </HelpLegend>
              <div className="grid gap-1.5 sm:grid-cols-2">
                {NOTIFICATION_EVENTS.map((e) => (
                  <Label key={e} className="flex items-center gap-2 rounded-lg border px-2.5 py-2 text-sm font-normal">
                    <Checkbox checked={draft.events.includes(e)} onCheckedChange={() => toggleEvent(e)} />
                    {t(`eventNames.${e}`)}
                  </Label>
                ))}
              </div>
            </FieldSet>
            <DialogFooter className="gap-2 sm:justify-between">
              {!channel ? (
                <Button type="button" variant="ghost" onClick={() => setKind(null)}>
                  <ArrowLeft data-icon="inline-start" />
                  {t("back")}
                </Button>
              ) : (
                <span />
              )}
              <Button type="submit">{t("apply")}</Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}

/**
 * Only configured channels are listed; the channel kind is picked when adding
 * one. Below the hub form sits the browser-local presentation of major actions.
 */
export function NotificationsSection({ settings }: { settings: HubSettings }) {
  const t = useTranslations("settings");
  const tn = useTranslations("settings.notifications");
  const dt = useAdapterText();
  const adapter = useAdapter();
  const capability = useCapability("testNotification");
  const save = useSaveSettings();
  const { draft, setDraft, dirty, reset, commit } = useSectionDraft(settings.notifications);
  const [editing, setEditing] = useState<NotificationChannel | "new" | null>(null);
  const [testing, setTesting] = useState<string | null>(null);
  if (!draft) return null;
  const links = notificationBaseUrl(settings.general);
  const update = (c: NotificationChannel) => setDraft(draft.some((x) => x.id === c.id) ? draft.map((x) => (x.id === c.id ? c : x)) : [...draft, c]);
  return (
    <div className="flex flex-col gap-5">
      <SettingsForm
        title={t("sections.notifications")}
        description={tn("onlyConfigured")}
        dirty={dirty}
        onDiscard={reset}
        onSave={async () => {
          await save("notifications", draft);
          commit(draft);
        }}
      >
        <p className="flex flex-wrap items-center gap-1.5 rounded-xl border bg-card/40 p-3 text-xs text-muted-foreground">
          <Link2 className="size-3.5 shrink-0" aria-hidden />
          {tn("linkBase", { url: links.url, source: links.source })}
          <Link to="/settings/general" className="text-primary-text hover:underline">
            {tn("linkBaseEdit")}
          </Link>
        </p>
        <div className="flex items-center justify-between gap-2">
          <h3 className="text-sm font-semibold">{tn("channels")}</h3>
          <Button type="button" size="sm" variant="outline" onClick={() => setEditing("new")}>
            <Plus data-icon="inline-start" />
            {tn("add")}
          </Button>
        </div>
        {draft.length === 0 ? (
          <p className="rounded-xl border border-dashed p-4 text-sm text-muted-foreground">{tn("empty")}</p>
        ) : (
          <ul className="flex flex-col gap-2">
            {draft.map((c) => (
              <li key={c.id} className={cn("flex flex-wrap items-center gap-3 rounded-xl border bg-card/40 p-3", !c.enabled && "opacity-70")}>
                <span className="grid size-9 place-items-center rounded-xl bg-primary/12 text-primary-text [&_svg]:size-4">
                  <KindIcon kind={c.kind} />
                </span>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium">{c.name}</span>
                    <ToneBadge tone="gray" className="h-5 px-1.5 text-[10.5px]">
                      {tn(`kinds.${c.kind}`)}
                    </ToneBadge>
                  </div>
                  <div className="truncate font-mono text-[11.5px] text-muted-foreground">{c.target}</div>
                  <div className="mt-1 flex flex-wrap gap-1">
                    {c.events.map((e) => (
                      <span key={e} className="rounded-md border px-1.5 py-0.5 text-[10.5px] text-muted-foreground">
                        {tn(`eventNames.${e}`)}
                      </span>
                    ))}
                  </div>
                </div>
                <div className="flex items-center gap-1.5">
                  <Switch aria-label={`${tn("enabled")}: ${c.name}`} checked={c.enabled} onCheckedChange={(enabled) => update({ ...c, enabled })} />
                  <HelpTarget topic="settingsNotifications.test">
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      disabled={!capability.available || testing === c.id} title={capability.reason}
                      onClick={async () => {
                        setTesting(c.id);
                        const r = await adapter.testNotification(c);
                        setTesting(null);
                        if (r.ok) toast.success(dt(r.message));
                        else toast.error(dt(r.message));
                      }}
                    >
                      {testing === c.id ? <Spinner data-icon="inline-start" /> : <Send data-icon="inline-start" />}
                      {testing === c.id ? tn("testing") : tn("test")}
                    </Button>
                  </HelpTarget>
                  <Button type="button" size="icon" variant="ghost" className="size-8" aria-label={`${tn("edit")}: ${c.name}`} onClick={() => setEditing(c)}>
                    <Pencil />
                  </Button>
                  <Button type="button" size="icon" variant="ghost" className="size-8 text-destructive hover:text-destructive" aria-label={`${tn("remove")}: ${c.name}`} onClick={() => setDraft(draft.filter((x) => x.id !== c.id))}>
                    <Trash2 />
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        )}
        {editing && (
          <ChannelDialog
            channel={editing === "new" ? null : editing}
            onClose={() => setEditing(null)}
            onApply={(c) => {
              update(c);
              setEditing(null);
            }}
          />
        )}
      </SettingsForm>
      <ActionPresentationPointer value={settings.activity.actionPresentation} />
    </div>
  );
}

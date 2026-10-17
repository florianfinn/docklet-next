import { KeyRound, LogOut, Palette, UserRound } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { LogoutDialog } from "@/app/shell/LogoutDialog";
import { useSettings } from "@/data/queries";
import { useAdapter } from "@/data/use-adapter";
import { TextField } from "@/shared/components/form-field";
import { PageHeader, QueryError } from "@/shared/components/page";
import { ToneBadge } from "@/shared/components/tone";
import { Avatar, AvatarFallback } from "@/shared/ui/avatar";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/shared/ui/card";
import { Skeleton } from "@/shared/ui/skeleton";

import { useAuthErrors } from "./field-rules";
import { ConnectedProfilePage } from "./ConnectedProfilePage";

/** Own profile and session; content follows the hub's login mode. */
export default function ProfilePage() {
  const adapter = useAdapter();
  return adapter.kind === "http" ? <ConnectedProfilePage /> : <DemoProfilePage />;
}

function DemoProfilePage() {
  const t = useTranslations("settings.profile");
  const tu = useTranslations("shell.user");
  const ta = useTranslations("settings.auth");
  const settings = useSettings();
  const [logout, setLogout] = useState(false);
  const [pw, setPw] = useState({ password: "", confirm: "" });
  const [changed, setChanged] = useState(false);
  const auth = settings.data?.auth;
  const errors = useAuthErrors(auth ? { ...auth, local: { ...auth.local, passwordSet: false } } : { mode: "none", local: { username: "", passwordSet: false }, proxy: { userHeader: "", emailHeader: "", groupsHeader: "", trustedProxies: "", logoutUrl: "" } }, pw);
  const initials = tu("name")
    .split(" ")
    .map((n) => n[0])
    .join("");

  return (
    <>
      <PageHeader title={t("title")} description={t("description")} />
      {settings.isError ? (
        <QueryError onRetry={() => void settings.refetch()} />
      ) : !auth ? (
        <Skeleton className="h-80 rounded-2xl" />
      ) : (
        <div className="grid max-w-6xl grid-cols-[minmax(0,1fr)] items-start gap-4 lg:grid-cols-2">
          <Card className="min-w-0 gap-4 py-5">
            <CardHeader className="px-5">
              <CardTitle className="flex items-center gap-2">
                <UserRound className="size-4 text-muted-foreground" aria-hidden />
                {t("account")}
              </CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-4 px-5">
              <div className="flex min-w-0 flex-wrap items-center gap-3">
                <Avatar className="size-12 rounded-xl">
                  <AvatarFallback className="rounded-xl bg-primary/20 font-semibold text-primary-text">{initials}</AvatarFallback>
                </Avatar>
                <div className="min-w-0 break-all">
                  <div className="font-semibold">{tu("name")}</div>
                  <div className="text-sm text-muted-foreground">{tu("email")}</div>
                </div>
                <ToneBadge tone="violet" className="ml-auto">
                  {tu("role")}
                </ToneBadge>
              </div>
              <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-sm">
                <dt className="text-muted-foreground">{t("session")}</dt>
                <dd>{auth.mode === "none" ? t("sessionNone") : auth.mode === "local" ? t("sessionLocal", { user: auth.local.username }) : t("sessionProxy", { header: auth.proxy.userHeader })}</dd>
              </dl>
              <div className="flex flex-wrap gap-2 border-t pt-3">
                <Button asChild variant="outline" size="sm">
                  <Link to="/settings/appearance">
                    <Palette data-icon="inline-start" />
                    {t("appearanceLink")}
                  </Link>
                </Button>
                <Button variant="ghost" size="sm" onClick={() => setLogout(true)}>
                  <LogOut data-icon="inline-start" />
                  {t("logout")}
                </Button>
              </div>
            </CardContent>
          </Card>
          <Card className="min-w-0 gap-4 py-5">
            <CardHeader className="px-5">
              <CardTitle className="flex items-center gap-2">
                <KeyRound className="size-4 text-muted-foreground" aria-hidden />
                {t("password")}
              </CardTitle>
              <CardDescription>{auth.mode === "local" ? ta("passwordHint") : t("passwordOnlyLocal")}</CardDescription>
            </CardHeader>
            {auth.mode === "local" && (
              <CardContent className="px-5">
                <form
                  className="flex flex-col gap-4"
                  onSubmit={(e) => {
                    e.preventDefault();
                    setChanged(true);
                    if (errors.password || errors.confirm) return;
                    setPw({ password: "", confirm: "" });
                    setChanged(false);
                    toast.success(t("passwordChanged"));
                  }}
                >
                  <TextField id="pf-pw" type="password" label={ta("password")} value={pw.password} onChange={(password) => setPw({ ...pw, password })} error={changed ? errors.password : undefined} autoComplete="new-password" />
                  <TextField id="pf-pw2" type="password" label={ta("passwordConfirm")} value={pw.confirm} onChange={(confirm) => setPw({ ...pw, confirm })} error={changed ? errors.confirm : undefined} autoComplete="new-password" />
                  <Button type="submit" className="self-start" disabled={!pw.password}>
                    {t("password")}
                  </Button>
                </form>
              </CardContent>
            )}
          </Card>
        </div>
      )}
      <LogoutDialog open={logout} onOpenChange={setLogout} />
    </>
  );
}

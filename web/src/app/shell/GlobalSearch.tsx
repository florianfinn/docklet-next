// Source: docklet-next/issues/356
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original role-scoped command palette using existing query consumers. M19 web consumer integration.

import { useCallback, useEffect, useState } from "react";
import { Search } from "lucide-react";
import { useNavigate } from "react-router";
import { useTranslations } from "use-intl";
import { useAuthMode, useStacks, useHosts } from "@/data/queries";
import { useIsUser } from "@/data/use-principal";
import { useAdapter } from "@/data/use-adapter";
import { identityLifetime } from "@/data/identity-lifetime";
import { setTokens } from "@/shared/theme/design-tokens";
import { useTokens } from "@/shared/theme/use-design-tokens";
import { Button } from "@/shared/ui/button";
import { Command, CommandInput, CommandList, CommandEmpty } from "@/shared/ui/command";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/shared/ui/dialog";
import { LogoutDialog } from "./LogoutDialog";
import { SearchGroup } from "./SearchGroup";
import { AdminSearchGroups } from "./AdminSearchGroups";
import { isSearchShortcut } from "./search-model";

export function GlobalSearch() {
  const t = useTranslations("shell.search"), adapter = useAdapter();
  const [open, setOpen] = useState(false), [logout, setLogout] = useState(false);
  useEffect(() => {
    const key = (event: KeyboardEvent) => { if (!isSearchShortcut(event)) return; event.preventDefault(); setOpen((value) => !value); };
    document.addEventListener("keydown", key);
    const off = identityLifetime(adapter).listen(() => { setOpen(false); setLogout(false); });
    return () => { document.removeEventListener("keydown", key); off(); };
  }, [adapter]);
  const close = useCallback(() => setOpen(false), []);
  const signOut = useCallback(() => { setOpen(false); setLogout(true); }, []);
  const apple = /Mac|iPhone|iPad/.test(navigator.platform);
  return <><Button size="sm" variant="ghost" aria-label={t("title")} onClick={() => setOpen(true)}><Search data-icon="inline-start" /><kbd className="hidden text-xs text-muted-foreground sm:inline">{apple ? "⌘" : "Ctrl"}+K</kbd></Button>
    <Dialog open={open} onOpenChange={setOpen}><DialogContent className="overflow-hidden p-0 sm:max-w-xl"><DialogHeader className="sr-only"><DialogTitle>{t("title")}</DialogTitle><DialogDescription>{t("hint")}</DialogDescription></DialogHeader>{open && <SearchContents close={close} signOut={signOut} />}</DialogContent></Dialog>
    {logout && <LogoutDialog open onOpenChange={setLogout} />}</>;
}
function SearchContents({ close, signOut }: { close: () => void; signOut: () => void }) {
  const t = useTranslations("shell.search"), nav = useTranslations("shell.nav");
  const user = useIsUser(), stacks = useStacks(), hosts = useHosts(), tokens = useTokens(), mode = useAuthMode();
  const [query, setQuery] = useState("");
  const navigate = useNavigate();
  const select = (to?: string, run?: () => void) => { close(); if (to) void navigate(to); else run?.(); };
  const hostName = (id: string) => hosts.data?.find((host) => host.id === id)?.name ?? id;
  const pages = [{ id: "dashboard", label: nav("dashboard"), to: "/dashboard" }, { id: "stacks", label: nav("stacks"), to: "/stacks" }, { id: "profile", label: nav("profile"), to: "/profile" }];
  const actions = [{ id: "theme", label: t("theme"), run: () => setTokens({ theme: tokens.theme === "dark" ? "light" : "dark" }) }, ...(["local", "proxy"].includes(mode.data ?? "") ? [{ id: "logout", label: t("signOut"), run: signOut }] : [])];
  return <Command shouldFilter={false}><CommandInput aria-label={t("title")} placeholder={t("placeholder")} value={query} onValueChange={setQuery} /><CommandList><CommandEmpty>{t("empty")}</CommandEmpty>
    {user ? <>
      <SearchGroup name={t("pages")} entries={pages} query={query} select={select} />
      <SearchGroup name={nav("stacks")} entries={stacks.data?.map((stack) => ({ id: stack.id, label: stack.name, host: hostName(stack.hostId), keywords: stack.labels, to: `/stacks/${encodeURIComponent(stack.id)}` })) ?? []} query={query} select={select} loading={stacks.isPending} list="/stacks" />
      <SearchGroup name={t("actions")} entries={actions} query={query} select={select} />
    </> : <AdminSearchGroups query={query} select={select} hostName={hostName} pages={pages} actions={actions} stacks={stacks.data ?? []} stacksLoading={stacks.isPending} />}
  </CommandList></Command>;
}

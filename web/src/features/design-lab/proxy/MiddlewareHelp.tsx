// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 1a4a628bcb7569360c8126b781275c14cfc51074
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import type { ReactNode } from "react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";
import { useProxyText } from "./use-proxy-text";
const HELP = { compress: "compressHelp", "strip-api": "stripHelp", "custom-headers": "headersHelp" } as const;
export function MiddlewareHelp({ name, children }: { name: string; children?: ReactNode }) {
  const t = useProxyText();
  const key = HELP[name as keyof typeof HELP] ?? "middlewareHelp";
  return <Tooltip><TooltipTrigger asChild>{children ?? <span tabIndex={0} className="cursor-help border-b border-dotted border-primary/60">{name}</span>}</TooltipTrigger><TooltipContent className="max-w-72"><p>{t(key)}</p></TooltipContent></Tooltip>;
}

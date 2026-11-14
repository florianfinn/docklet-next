// Source: docklet-next/issues/449
// Revision: db966bd5e74458ea032d70eba1b7e4593c7b104e
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original shared-trigger tooltip and tap help composition using existing free primitives.

import { useRef, useState } from "react";
import { CircleHelp } from "lucide-react";
import { Button } from "@/shared/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/shared/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

export function HelpTap({ label, help }: { label: string; help: string }) {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  return <Popover open={open} onOpenChange={setOpen}>
    <Tooltip open={open ? false : undefined}>
      <TooltipTrigger asChild><PopoverTrigger asChild>
        <Button ref={trigger} type="button" variant="ghost" size="icon-xs" aria-label={label}><CircleHelp aria-hidden /></Button>
      </PopoverTrigger></TooltipTrigger>
      <TooltipContent className="max-w-xs">{help}</TooltipContent>
    </Tooltip>
    <PopoverContent onCloseAutoFocus={(event) => { event.preventDefault(); trigger.current?.focus(); }} aria-label={label} className="max-w-xs text-sm" onClick={(event) => event.stopPropagation()}>{help}</PopoverContent>
  </Popover>;
}

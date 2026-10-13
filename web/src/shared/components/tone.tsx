import type { ComponentProps, ReactNode } from "react";

import type { Tone } from "@/domain/model";
import { cn } from "@/shared/lib/utils";
import { Badge } from "@/shared/ui/badge";

/** Badge tinted by a semantic tone token (Arcane pill style). */
export function ToneBadge({ tone, className, children, ...props }: ComponentProps<typeof Badge> & { tone: Tone }) {
  return (
    <Badge
      variant="outline"
      data-tone={tone}
      className={cn("h-6 gap-1.5 rounded-lg border-(--tone)/30 bg-(--tone)/12 px-2.5 text-(--tone-text)", className)}
      {...props}
    >
      {children}
    </Badge>
  );
}

export function ToneDot({ tone, pulse, className, label }: { tone: Tone; pulse?: boolean; className?: string; label?: string }) {
  return (
    <span
      data-tone={tone}
      role={label ? "img" : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
      className={cn("inline-block size-2 shrink-0 rounded-full bg-(--tone)", pulse && "animate-pulse-ring", className)}
    />
  );
}

/** Rounded icon tile tinted by tone or an explicit colour (host colours). */
export function IconTile({
  tone,
  color,
  size = "md",
  className,
  children,
}: {
  tone?: Tone;
  color?: string;
  size?: "sm" | "md" | "lg";
  className?: string;
  children: ReactNode;
}) {
  return (
    <span
      data-tone={tone ?? "accent"}
      style={color ? ({ "--tone": color } as React.CSSProperties) : undefined}
      className={cn(
        "grid shrink-0 place-items-center bg-(--tone)/14 text-(--tone)",
        size === "sm" && "size-7 rounded-lg [&_svg]:size-3.5",
        size === "md" && "size-9 rounded-xl [&_svg]:size-4",
        size === "lg" && "size-11 rounded-xl [&_svg]:size-5",
        className,
      )}
    >
      {children}
    </span>
  );
}

/** Round gradient icon with a soft glow; glow strength follows the --glow token. */
export function LuminousIcon({ tone, children, className }: { tone: Tone; children: ReactNode; className?: string }) {
  return (
    <span data-tone={tone} className={cn("lum-icon grid size-10 shrink-0 place-items-center rounded-full text-white [&_svg]:size-5", className)}>
      {children}
    </span>
  );
}

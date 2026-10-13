import { useId } from "react";

import type { ServiceKind } from "../dashboard-layout";

// Own simplified marks in the services' typical colours; not the official logos.

function PlexMark() {
  return (
    <svg viewBox="0 0 32 32" aria-hidden className="size-full">
      <rect width="32" height="32" rx="8" fill="#1f1f1f" />
      <path d="M11.5 7h5.4l6.6 9-6.6 9h-5.4l6.6-9z" fill="#e5a00d" />
    </svg>
  );
}

function MinecraftMark() {
  return (
    <svg viewBox="0 0 32 32" aria-hidden className="size-full">
      <rect width="32" height="32" rx="8" fill="#2d2a26" />
      <path d="M16 5.5 26 11v10.5L16 27 6 21.5V11z" fill="#866043" />
      <path d="M16 5.5 26 11l-10 5.5L6 11z" fill="#6aa84f" />
      <path d="M6 11l10 5.5v3L6 14z" fill="#5b8c3e" />
      <path d="M26 11 16 16.5v3L26 14z" fill="#4e7a34" />
      <path d="M16 16.5V27l10-5.5V11z" fill="#000" opacity=".18" />
    </svg>
  );
}

function OverseerrMark() {
  const id = `ovr-${useId().replace(/:/g, "")}`;
  return (
    <svg viewBox="0 0 32 32" aria-hidden className="size-full">
      <defs>
        <linearGradient id={id} x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#6366f1" />
          <stop offset="1" stopColor="#a855f7" />
        </linearGradient>
      </defs>
      <rect width="32" height="32" rx="8" fill="#141827" />
      <circle cx="16" cy="16" r="9.5" fill="none" stroke={`url(#${id})`} strokeWidth="3.4" />
      <circle cx="18.6" cy="13.6" r="3.4" fill={`url(#${id})`} />
    </svg>
  );
}

const MARKS: Record<ServiceKind, () => React.JSX.Element> = { plex: PlexMark, minecraft: MinecraftMark, overseerr: OverseerrMark };

export function AppIcon({ kind, className }: { kind: ServiceKind; className?: string }) {
  const Mark = MARKS[kind];
  return (
    <span className={className ?? "grid size-7 shrink-0 overflow-hidden rounded-lg"} data-app={kind}>
      <Mark />
    </span>
  );
}

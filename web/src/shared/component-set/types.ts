import type { ReactNode } from "react";

import type { Tone } from "@/domain/model";

// Prop contracts of the shared building blocks; callers only depend on these.

export interface SparklineProps {
  data: number[];
  type?: "line" | "bar" | "area";
  color?: string;
  /** Fixed domain, e.g. [0, 100] for percentages; auto otherwise. */
  domain?: [number, number];
  animate?: boolean;
  label?: string;
  className?: string;
}

export interface StripItem {
  label: string;
  value: ReactNode;
  icon?: ReactNode;
  sub?: ReactNode;
  tone?: Tone;
}

export interface StatStripProps {
  items: StripItem[];
  className?: string;
}

export interface BulkActionBarProps {
  count: number;
  selectedLabel: string;
  clearLabel: string;
  onClear: () => void;
  children: ReactNode;
  className?: string;
}

export interface ProgressItem {
  /** Rows render with id `run-<id>` so the action centre can scroll to them. */
  id: string;
  title: ReactNode;
  description: ReactNode;
  progress: number;
  tone: Tone;
  media?: ReactNode;
  actions?: ReactNode;
  body?: ReactNode;
  highlighted?: boolean;
}

export interface ProgressListProps {
  items: ProgressItem[];
  className?: string;
}

export interface InfoHoverCardProps {
  trigger: ReactNode;
  icon: ReactNode;
  title: string;
  children: ReactNode;
  tone?: Tone;
  className?: string;
}

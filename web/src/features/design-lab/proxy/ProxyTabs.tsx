import type { ReactNode } from "react";
import { TabsList, TabsTrigger } from "@/shared/ui/tabs";

/** Navigation tabs share Docklet's control colour and minimal toggle edge light. */
export function ProxyTabs({ options, label }: { options: { value: string; label: ReactNode }[]; label?: string }) {
  return <TabsList variant="line" aria-label={label} className="mb-4 max-w-full flex-wrap justify-start gap-0 border-b p-0 group-data-[orientation=horizontal]/tabs:h-auto">{options.map((option) => <TabsTrigger key={option.value} value={option.value} className="h-9 flex-none rounded-none px-3 data-[state=active]:text-primary-text dark:data-[state=active]:text-primary-text after:bottom-[-1px] after:bg-(--control) after:shadow-[0_0_8px_var(--highlight)]">{option.label}</TabsTrigger>)}</TabsList>;
}

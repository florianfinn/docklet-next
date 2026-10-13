import { cn } from "@/shared/lib/utils"

// Adapted from shadcn/ui skeleton: the fill uses --skeleton (secondary palette role) instead of --accent.

function Skeleton({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="skeleton"
      className={cn("animate-pulse rounded-md bg-skeleton", className)}
      {...props}
    />
  )
}

export { Skeleton }

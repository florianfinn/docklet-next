import { CircleCheckIcon, InfoIcon, Loader2Icon, OctagonXIcon, TriangleAlertIcon } from "lucide-react"
import { Toaster as Sonner, type ToasterProps } from "sonner"

// Adapted from shadcn/ui sonner: theme comes from props instead of next-themes;
// the toast width follows --toast-scale. Toasts stay expanded, so several
// notices stack downwards and stay readable; styling lives in notifications.css.
// Sonner keeps no timer: a notice's CSS countdown bar is its lifetime (see notice-life.ts).
const Toaster = ({ style, ...props }: ToasterProps) => {
  return (
    <Sonner
      className="toaster group"
      expand
      closeButton
      duration={Number.POSITIVE_INFINITY}
      icons={{
        success: <CircleCheckIcon className="size-4" />,
        info: <InfoIcon className="size-4" />,
        warning: <TriangleAlertIcon className="size-4" />,
        error: <OctagonXIcon className="size-4" />,
        loading: <Loader2Icon className="size-4 animate-spin" />,
      }}
      style={
        {
          "--normal-bg": "var(--popover)",
          "--normal-text": "var(--popover-foreground)",
          "--normal-border": "var(--border)",
          "--border-radius": "var(--radius)",
          // Toast width follows the design-lab size; on phones it fills the width between sonner's 8 px offsets.
          "--width": "min(calc(23rem * var(--toast-scale, 1.3)), calc(100vw - 1rem))",
          ...style,
        } as React.CSSProperties
      }
      {...props}
    />
  )
}

export { Toaster }

import { cn } from "@/shared/lib/utils"
import { Loader2Icon } from "lucide-react"
import { useTranslations } from "use-intl"

function Spinner({ className, ...props }: React.ComponentProps<"svg">) {
  const t = useTranslations("locale.ui")
  return (
    <Loader2Icon
      role="status"
      aria-label={t("loading")}
      className={cn("size-4 animate-spin", className)}
      {...props}
    />
  )
}

export { Spinner }

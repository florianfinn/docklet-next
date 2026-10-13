import { Eye, EyeOff } from "lucide-react";
import { useState, type ComponentProps } from "react";
import { useTranslations } from "use-intl";

import { cn } from "@/shared/lib/utils";
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/shared/ui/input-group";

/**
 * Password or secret input with a show/hide toggle (aria-pressed). The value
 * lives only in the caller's state; nothing is logged or stored here. A
 * mouse click on the toggle keeps the focus and caret in the field; keyboard
 * users reach the toggle with Tab. aria-invalid and aria-describedby stay on
 * the input, so error messages remain attached while the value is visible.
 */
export function PasswordInput({ className, disabled, ...props }: Omit<ComponentProps<"input">, "type">) {
  const t = useTranslations("common.secret");
  const [shown, setShown] = useState(false);
  return (
    <InputGroup data-disabled={disabled || undefined} className={cn("min-w-0 flex-1", className)}>
      <InputGroupInput {...props} disabled={disabled} type={shown ? "text" : "password"} spellCheck={false} data-secret-visible={shown || undefined} />
      <InputGroupAddon align="inline-end">
        <InputGroupButton
          size="icon-xs"
          aria-pressed={shown}
          aria-label={shown ? t("hide") : t("show")}
          aria-controls={props.id}
          title={shown ? t("hide") : t("show")}
          disabled={disabled}
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => setShown((s) => !s)}
        >
          {shown ? <EyeOff /> : <Eye />}
        </InputGroupButton>
      </InputGroupAddon>
    </InputGroup>
  );
}

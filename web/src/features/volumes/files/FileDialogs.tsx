import { useState } from "react";
import { useTranslations } from "use-intl";

import { validateFileName } from "@/domain/files";
import { TextField } from "@/shared/components/form-field";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Spinner } from "@/shared/ui/spinner";

interface NameDialogProps {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  title: string;
  submitLabel: string;
  initial?: string;
  siblings: string[];
  onSubmit: (name: string) => Promise<boolean>;
}

/** Name input for new files, new folders and renames, validated against siblings. */
export function NameDialog(props: NameDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-md">{props.open && <NameForm key={props.initial ?? ""} {...props} />}</DialogContent>
    </Dialog>
  );
}

function NameForm({ onOpenChange, title, submitLabel, initial = "", siblings, onSubmit }: NameDialogProps) {
  const t = useTranslations("volumes.files");
  const [name, setName] = useState(initial);
  const [tried, setTried] = useState(false);
  const [busy, setBusy] = useState(false);
  const error = validateFileName(name, siblings, initial || undefined);
  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={async (e) => {
        e.preventDefault();
        setTried(true);
        if (error) return;
        setBusy(true);
        const ok = await onSubmit(name.trim());
        setBusy(false);
        if (ok) onOpenChange(false);
      }}
    >
      <DialogHeader>
        <DialogTitle>{title}</DialogTitle>
        <DialogDescription>{t("deleteDemo")}</DialogDescription>
      </DialogHeader>
      <TextField id="file-name" label={t("nameLabel")} value={name} onChange={setName} error={tried && error ? t(`nameErrors.${error}`) : undefined} mono />
      <DialogFooter>
        <Button type="submit" disabled={busy}>
          {busy && <Spinner data-icon="inline-start" />}
          {submitLabel}
        </Button>
      </DialogFooter>
    </form>
  );
}

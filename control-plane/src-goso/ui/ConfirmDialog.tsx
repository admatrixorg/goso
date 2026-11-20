import { useI18n } from "../i18n";
import { Button } from "./Button";
import { Dialog, DialogFooter, DialogHeader } from "./Dialog";

export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel,
  variant = "default",
  onConfirm,
  loading,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  confirmLabel?: string;
  variant?: "default" | "destructive";
  onConfirm: () => void;
  loading?: boolean;
}) {
  const { t } = useI18n();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogHeader title={title} description={description} />
      <DialogFooter>
        <Button variant="outline" onClick={() => onOpenChange(false)} disabled={loading}>
          {t("common.cancel")}
        </Button>
        <Button variant={variant} onClick={onConfirm} disabled={loading}>
          {loading ? t("common.loading") : (confirmLabel ?? t("common.save"))}
        </Button>
      </DialogFooter>
    </Dialog>
  );
}

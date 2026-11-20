import { useEffect, useState } from "react";
import { namedConfirmTarget } from "../api/confirm";
import { useI18n } from "../i18n";
import { Button } from "./Button";
import { Dialog, DialogFooter, DialogHeader } from "./Dialog";

export function ConfirmDeleteDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmValue,
  confirmLabel,
  onConfirm,
  loading,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  confirmValue: string;
  confirmLabel?: string;
  onConfirm: () => void;
  loading?: boolean;
}) {
  const { t } = useI18n();
  const [typed, setTyped] = useState("");
  const target = confirmValue.trim();

  useEffect(() => {
    if (!open) setTyped("");
  }, [open]);

  const ok = namedConfirmTarget(target, typed);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogHeader title={title} description={description} />
      <div>
        <p className="z-dialog-desc" style={{ marginBottom: 8 }}>
          {t("common.typeToConfirmPrefix")} <strong style={{ color: "var(--foreground)" }}>{target}</strong> {t("common.typeToConfirmSuffix")}
        </p>
        <input
          className="z-field"
          style={{ width: "100%" }}
          value={typed}
          onChange={(e) => setTyped(e.target.value)}
          placeholder={target}
          autoFocus
        />
      </div>
      <DialogFooter>
        <Button variant="outline" onClick={() => onOpenChange(false)} disabled={loading}>
          {t("common.cancel")}
        </Button>
        <Button variant="destructive" onClick={onConfirm} disabled={!ok || loading}>
          {loading ? t("common.loading") : (confirmLabel ?? t("common.delete"))}
        </Button>
      </DialogFooter>
    </Dialog>
  );
}

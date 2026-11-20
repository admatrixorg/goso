import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useTranslation } from "react-i18next";
import { AlertCircle } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { emailFormSchema, type EmailFormData } from "@/schemas/login.schema";
import { LOCAL_STORAGE_KEYS } from "@/lib/constants";

export interface EmailLoginResult {
  token: string;
  userId: string;
  email?: string;
  displayName?: string;
  tenantId?: string;
  tenantSlug?: string;
  tenantName?: string;
  role?: string;
}

interface EmailFormProps {
  onSubmit: (result: EmailLoginResult) => void;
}

export function EmailForm({ onSubmit }: EmailFormProps) {
  const { t } = useTranslation("login");
  const [connecting, setConnecting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<EmailFormData>({
    resolver: zodResolver(emailFormSchema),
    defaultValues: { email: "", password: "" },
  });

  const onValid = async (data: EmailFormData) => {
    setConnecting(true);
    setError(null);
    try {
      const res = await fetch("/v1/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          email: data.email.trim(),
          password: data.password,
        }),
      });
      const body = await res.json().catch(() => ({}));
      if (res.status === 401) {
        setError(t("email.errorInvalidCredentials"));
        return;
      }
      if (res.status === 423) {
        setError(t("email.errorLocked"));
        return;
      }
      if (!res.ok) {
        const msg = typeof body.error === "string" ? body.error : t("email.errorServer", { status: res.status });
        setError(msg);
        return;
      }
      const token = body.access_token as string | undefined;
      const userId = body.user?.id as string | undefined;
      if (!token || !userId) {
        setError(t("email.errorServer", { status: res.status }));
        return;
      }
      const slug = body.tenant?.slug as string | undefined;
      if (slug) {
        localStorage.setItem(LOCAL_STORAGE_KEYS.TENANT_ID, slug);
      }
      onSubmit({
        token,
        userId,
        email: body.user?.email,
        displayName: body.user?.email,
        tenantId: body.tenant?.id,
        tenantSlug: slug,
        tenantName: body.tenant?.name,
        role: body.tenant?.role,
      });
    } catch {
      setError(t("email.errorCannotConnect"));
    } finally {
      setConnecting(false);
    }
  };

  return (
    <form onSubmit={handleSubmit(onValid)} className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="email" className="text-sm font-medium">
          {t("email.email")}
        </Label>
        <Input
          id="email"
          type="email"
          autoComplete="username"
          {...register("email")}
          placeholder={t("email.emailPlaceholder")}
          className="text-base md:text-sm"
          autoFocus
          disabled={connecting}
        />
        {errors.email && <p className="text-xs text-destructive">{errors.email.message}</p>}
      </div>
      <div className="space-y-2">
        <Label htmlFor="password" className="text-sm font-medium">
          {t("email.password")}
        </Label>
        <Input
          id="password"
          type="password"
          autoComplete="current-password"
          {...register("password")}
          placeholder={t("email.passwordPlaceholder")}
          className="text-base md:text-sm"
          disabled={connecting}
        />
        {errors.password && <p className="text-xs text-destructive">{errors.password.message}</p>}
      </div>
      {error && (
        <div className="flex items-start gap-2 rounded-md border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
          <span>{error}</span>
        </div>
      )}
      <button
        type="submit"
        disabled={connecting}
        className="inline-flex h-9 w-full items-center justify-center rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground shadow transition-colors hover:bg-primary/90 disabled:pointer-events-none disabled:opacity-50"
      >
        {connecting ? t("email.connecting") : t("email.signIn")}
      </button>
    </form>
  );
}

import { Eye, EyeOff } from "lucide-react";
import { type ComponentProps, useState } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { cn } from "@/foundation/lib/utils";

export function PasswordField({
  className,
  disabled,
  ...props
}: Omit<ComponentProps<typeof Input>, "type">) {
  const { t } = useTranslation();
  const [visible, setVisible] = useState(false);
  const Icon = visible ? EyeOff : Eye;
  return (
    <div className="relative">
      <Input
        {...props}
        disabled={disabled}
        type={visible ? "text" : "password"}
        className={cn(className, "pr-12")}
      />
      <button
        type="button"
        disabled={disabled}
        aria-pressed={visible}
        aria-label={t(visible ? "auth.hidePassword" : "auth.showPassword")}
        className="absolute right-2 top-1/2 grid h-8 w-8 -translate-y-1/2 place-items-center rounded-full text-muted-foreground hover:bg-muted focus-visible:outline focus-visible:outline-2 disabled:opacity-50"
        onClick={() => setVisible((value) => !value)}
      >
        <Icon className="h-4 w-4" />
      </button>
    </div>
  );
}

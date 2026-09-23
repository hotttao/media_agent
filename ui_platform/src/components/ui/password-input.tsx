import { Eye, EyeOff } from "lucide-react";
import * as React from "react";
import { Input } from "@/components/ui/input";
import { cn } from "@/foundation/lib/utils";

const PasswordInput = React.forwardRef<
  HTMLInputElement,
  React.ComponentProps<"input">
>(({ className, disabled, id, ...props }, ref) => {
  const [isVisible, setIsVisible] = React.useState(false);
  const actionLabel = isVisible ? "Hide password" : "Show password";

  return (
    <div className="relative">
      <Input
        ref={ref}
        id={id}
        type={isVisible ? "text" : "password"}
        className={cn("pr-10", className)}
        disabled={disabled}
        {...props}
      />
      <button
        type="button"
        className="absolute inset-y-0 right-0 flex w-10 items-center justify-center rounded-r-md text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50"
        aria-label={actionLabel}
        aria-controls={id}
        aria-pressed={isVisible}
        disabled={disabled}
        onClick={() => setIsVisible((visible) => !visible)}
      >
        {isVisible ? (
          <EyeOff className="h-4 w-4" aria-hidden="true" />
        ) : (
          <Eye className="h-4 w-4" aria-hidden="true" />
        )}
      </button>
    </div>
  );
});

PasswordInput.displayName = "PasswordInput";

export { PasswordInput };

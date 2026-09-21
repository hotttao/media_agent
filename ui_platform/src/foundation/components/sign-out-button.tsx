import { useState } from "react";
import { Button, type ButtonProps } from "@/components/ui/button";
import { useAuth } from "@/foundation/providers/auth-provider";

interface SignOutButtonProps
  extends Pick<ButtonProps, "className" | "size" | "variant"> {
  label?: string;
}

export function SignOutButton({
  className,
  label = "Sign out",
  size = "sm",
  variant = "ghost",
}: SignOutButtonProps) {
  const { logout } = useAuth();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  async function handleSignOut() {
    if (pending) return;
    setPending(true);
    setError("");
    try {
      await logout();
    } catch (cause) {
      setPending(false);
      setError(cause instanceof Error ? cause.message : "Unable to sign out");
    }
  }

  return (
    <div className="relative">
      <Button
        type="button"
        className={className}
        variant={variant}
        size={size}
        disabled={pending}
        aria-busy={pending}
        onClick={() => void handleSignOut()}
      >
        {pending ? "Signing out…" : label}
      </Button>
      {error && (
        <p
          role="alert"
          className="absolute right-0 top-full z-50 mt-2 w-64 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 shadow-lg"
        >
          {error}
        </p>
      )}
    </div>
  );
}

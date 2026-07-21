import { Loader2 } from "lucide-react";
import { cn } from "@/foundation/lib/utils";

interface SpinnerProps extends React.SVGProps<SVGSVGElement> {
  size?: "sm" | "md" | "lg";
}

const sizeMap = { sm: "h-4 w-4", md: "h-6 w-6", lg: "h-8 w-8" };

export function Spinner({ className, size = "md", ...props }: SpinnerProps) {
  return <Loader2 className={cn("animate-spin", sizeMap[size], className)} {...props} />;
}

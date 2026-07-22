import { Badge } from "@/components/ui/badge";
import { cn } from "@/foundation/lib/utils";
import type { ProductStatus } from "@/domains/products/types";

const STATUS_STYLE: Record<ProductStatus, string> = {
  active: "bg-success/15 text-success",
  archived: "bg-muted text-muted-foreground",
};

export function StatusBadge({ status }: { status: ProductStatus }) {
  return (
    <Badge className={cn("font-medium capitalize", STATUS_STYLE[status])}>
      {status}
    </Badge>
  );
}

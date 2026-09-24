import { Badge } from "@/components/ui/badge";
import { useTranslation } from "react-i18next";
import { cn } from "@/foundation/lib/utils";
import type { Visibility } from "@/domains/materials/types";

const VISIBILITY_STYLE: Record<Visibility, string> = {
  private: "bg-muted text-muted-foreground",
  team: "bg-info/15 text-info",
  public: "bg-success/15 text-success",
};

export function VisibilityBadge({ visibility }: { visibility: Visibility }) {
  const { t } = useTranslation();
  return (
    <Badge
      className={cn("font-medium capitalize", VISIBILITY_STYLE[visibility])}
    >
      {t(`common.visibility.${visibility}`)}
    </Badge>
  );
}

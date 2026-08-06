import type { NavigationItem } from "@/foundation/types/navigation";

export const navigationItems: readonly NavigationItem[] = [
  {
    id: "materials",
    path: "/materials",
    labelKey: "navigation.materials",
    group: "library",
  },
  {
    id: "generatedAssets",
    path: "/generated-assets",
    labelKey: "navigation.generatedAssets",
    group: "library",
  },
  {
    id: "virtualIps",
    path: "/virtual-ips",
    labelKey: "navigation.virtualIps",
    group: "identity",
  },
  {
    id: "products",
    path: "/products",
    labelKey: "navigation.products",
    group: "identity",
  },
  {
    id: "assetKinds",
    path: "/settings/asset-kinds",
    labelKey: "navigation.assetKinds",
    group: "management",
  },
] as const;

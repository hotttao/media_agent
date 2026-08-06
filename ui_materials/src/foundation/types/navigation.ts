export type NavigationItemId =
  | "materials"
  | "generatedAssets"
  | "virtualIps"
  | "products"
  | "assetKinds";

export interface NavigationItem {
  id: NavigationItemId;
  path: string;
  labelKey: string;
  group: "library" | "identity" | "management";
}

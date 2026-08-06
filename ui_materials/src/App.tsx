import { Navigate, Route, Routes } from "react-router-dom";
import { ApplicationShell } from "@/foundation/components/application-shell";
import { AssetKindCreatePage } from "@/pages/asset-kind/create";
import { AssetKindEditPage } from "@/pages/asset-kind/edit";
import { AssetKindListPage } from "@/pages/asset-kind/list";
import { AssetKindShowPage } from "@/pages/asset-kind/show";
import { GeneratedAssetListPage } from "@/pages/generated-asset/list";
import { GeneratedAssetShowPage } from "@/pages/generated-asset/show";
import { MaterialCreatePage } from "@/pages/material/create";
import { MaterialEditPage } from "@/pages/material/edit";
import { MaterialListPage } from "@/pages/material/list";
import { MaterialShowPage } from "@/pages/material/show";
import { ProductCreatePage } from "@/pages/product/create";
import { ProductEditPage } from "@/pages/product/edit";
import { ProductListPage } from "@/pages/product/list";
import { ProductShowPage } from "@/pages/product/show";
import { VirtualIpCreatePage } from "@/pages/virtual-ip/create";
import { VirtualIpEditPage } from "@/pages/virtual-ip/edit";
import { VirtualIpListPage } from "@/pages/virtual-ip/list";
import { VirtualIpShowPage } from "@/pages/virtual-ip/show";

/** Composition root shared by the standalone and plugin routers. */
export default function App() {
  return (
    <ApplicationShell>
      <Routes>
        <Route path="/" element={<Navigate to="/materials" replace />} />
        <Route path="/materials" element={<MaterialListPage />} />
        <Route path="/materials/new" element={<MaterialCreatePage />} />
        <Route path="/materials/:id" element={<MaterialShowPage />} />
        <Route path="/materials/:id/edit" element={<MaterialEditPage />} />
        <Route path="/generated-assets" element={<GeneratedAssetListPage />} />
        <Route
          path="/generated-assets/:id"
          element={<GeneratedAssetShowPage />}
        />
        <Route path="/virtual-ips" element={<VirtualIpListPage />} />
        <Route path="/virtual-ips/new" element={<VirtualIpCreatePage />} />
        <Route path="/virtual-ips/:id" element={<VirtualIpShowPage />} />
        <Route path="/virtual-ips/:id/edit" element={<VirtualIpEditPage />} />
        <Route path="/products" element={<ProductListPage />} />
        <Route path="/products/new" element={<ProductCreatePage />} />
        <Route path="/products/:id" element={<ProductShowPage />} />
        <Route path="/products/:id/edit" element={<ProductEditPage />} />
        <Route path="/settings/asset-kinds" element={<AssetKindListPage />} />
        <Route
          path="/settings/asset-kinds/new"
          element={<AssetKindCreatePage />}
        />
        <Route
          path="/settings/asset-kinds/:name"
          element={<AssetKindShowPage />}
        />
        <Route
          path="/settings/asset-kinds/:name/edit"
          element={<AssetKindEditPage />}
        />
        <Route path="*" element={<Navigate to="/materials" replace />} />
      </Routes>
    </ApplicationShell>
  );
}

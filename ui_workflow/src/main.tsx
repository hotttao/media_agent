/**
 * Standalone entry. Provider stack mirrors agent-platform/web/src/main.tsx
 * (QueryClient > Theme > Auth) but drops BrowserRouter (standalone has no
 * routes - OAuth callback 302s back to /, and the shell uses slot+app, not
 * URL routing). 401 redirects to /api/v1/auth/login (OAuth via media_auth).
 *
 * Plugin mode does NOT use this file - it builds src/plugin.tsx as an IIFE.
 */
import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { configureApi } from "@/foundation/lib/api-client";
import "@/foundation/lib/i18n"; // side-effect: initialize i18next singleton
import { AuthProvider } from "@/foundation/providers/auth-provider";
import { QueryProvider } from "@/foundation/providers/query-provider";
import { ThemeProvider } from "@/foundation/providers/theme-provider";
import "./index.css";

configureApi({
  onUnauthorized: () => {
    if (window.location.pathname !== "/login") {
      window.location.href = `/api/auth/login?next=${encodeURIComponent(window.location.pathname)}`;
    }
  },
});

const rootElement = document.getElementById("root");
if (!rootElement) {
  throw new Error("Root element #root not found in index.html");
}

ReactDOM.createRoot(rootElement).render(
  <React.StrictMode>
    <QueryProvider>
      <ThemeProvider defaultTheme="dark">
        <AuthProvider>
          <TooltipProvider>
            <App />
            <Toaster />
          </TooltipProvider>
        </AuthProvider>
      </ThemeProvider>
    </QueryProvider>
  </React.StrictMode>,
);

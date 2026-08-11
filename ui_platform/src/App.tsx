/**
 * Top-level App shell.
 *
 * Routes:
 *   /login            - LoginPage (public)
 *   /register         - RegisterPage (public)
 *   /oauth/authorize  - OAuthAuthorizePage (authenticated)
 *   /                 - DashboardPage (authenticated)
 *   *                 - redirect to /
 */

import { Navigate, Route, Routes } from "react-router-dom";
import { ProtectedRoute } from "@/foundation/components/protected-route";
import { LoginPage } from "@/pages/auth/login";
import { RegisterPage } from "@/pages/auth/register";
import { ChatPage } from "@/pages/chat";
import { OAuthAuthorizePage } from "@/pages/oauth/authorize";

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<RegisterPage />} />
      <Route
        path="/oauth/authorize"
        element={
          <ProtectedRoute>
            <OAuthAuthorizePage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/"
        element={
          <ProtectedRoute>
            <ChatPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/w/:workspaceId"
        element={
          <ProtectedRoute>
            <ChatPage />
          </ProtectedRoute>
        }
      />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

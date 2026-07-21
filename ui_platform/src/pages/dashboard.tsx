/**
 * Dashboard placeholder - shows current user info + logout.
 *
 * This is a minimal landing page. Real feature pages will replace this.
 */
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useAuth } from "@/foundation/providers/auth-provider";
import { useNavigate } from "react-router-dom";

export function DashboardPage() {
  const { user, logout } = useAuth();
  const navigate = useNavigate();

  const handleLogout = async () => {
    await logout();
    navigate("/login");
  };

  return (
    <div className="min-h-screen bg-background">
      <header className="border-b px-6 py-4 flex items-center justify-between">
        <h1 className="text-lg font-semibold">Media Platform</h1>
        <Button variant="outline" size="sm" onClick={handleLogout}>
          Logout
        </Button>
      </header>
      <main className="px-6 py-6">
        <Card className="max-w-2xl">
          <CardHeader>
            <CardTitle>Welcome, {user?.username}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <div>
              <span className="font-medium">ID:</span> {user?.id}
            </div>
            <div>
              <span className="font-medium">Role:</span> {user?.role}
            </div>
            <div>
              <span className="font-medium">Auth Source:</span> {user?.auth_source}
            </div>
            <div>
              <span className="font-medium">Auto Evolution:</span> {user?.auto_evolution ? "enabled" : "disabled"}
            </div>
          </CardContent>
        </Card>
      </main>
    </div>
  );
}

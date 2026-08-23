import { ArrowLeft, Plus, Search, Shield, Trash2, Users } from "lucide-react";
import { type FormEvent, useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, useNavigate } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { canSoftDeleteUser, statusTone } from "@/domains/user/lib/user-status";
import { api } from "@/foundation/lib/api-client";
import { useAuth } from "@/foundation/providers/auth-provider";
import type { UserRecord } from "@/foundation/types/auth";

export function UsersListPage() {
  const { t } = useTranslation();
  const { user, logout } = useAuth();
  const navigate = useNavigate();
  const [items, setItems] = useState<UserRecord[]>([]);
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [creating, setCreating] = useState(false);
  const [deleting, setDeleting] = useState<UserRecord | null>(null);

  const load = useCallback(
    async (q = query) => {
      setLoading(true);
      setError("");
      try {
        const page = await api.listUsers({ q, page: 1, page_size: 100 });
        setItems(page.items);
      } catch (cause) {
        setError(
          cause instanceof Error ? cause.message : t("users.errors.load"),
        );
      } finally {
        setLoading(false);
      }
    },
    [query, t],
  );

  useEffect(() => {
    void load("");
  }, [load]);
  if (user?.role !== "admin") return <Navigate to="/" replace />;

  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    await api.createUser({
      username: String(form.get("username")),
      display_name: String(form.get("display_name")),
      password: String(form.get("password")),
      email: String(form.get("email")) || undefined,
      role: String(form.get("role")),
    });
    setCreating(false);
    await load();
  }

  async function confirmDelete() {
    if (!deleting) return;
    try {
      await api.deleteUser(deleting.id);
      setDeleting(null);
      await load();
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : t("users.errors.delete"),
      );
      setDeleting(null);
    }
  }

  return (
    <main
      className="flex min-h-screen flex-col bg-slate-50 text-slate-950"
      data-testid="users-page"
    >
      <header className="flex h-12 items-center gap-3 border-b bg-white px-4">
        <button
          type="button"
          aria-label={t("users.actions.back")}
          onClick={() => navigate("/")}
        >
          <ArrowLeft size={17} />
        </button>
        <Shield size={17} />
        <strong className="text-sm">{t("users.title")}</strong>
        <Button
          className="ml-auto"
          variant="ghost"
          size="sm"
          onClick={() => void logout().then(() => navigate("/login"))}
        >
          {t("users.actions.signOut")}
        </Button>
      </header>
      <section className="mx-auto w-full max-w-6xl flex-1 p-5 sm:p-8">
        <div className="mb-6 flex flex-wrap items-end justify-between gap-4">
          <div>
            <h1 className="flex items-center gap-2 text-2xl font-semibold">
              <Users size={23} />
              {t("users.heading")}
            </h1>
            <p className="mt-1 text-sm text-slate-500">
              {t("users.description")}
            </p>
          </div>
          <Button onClick={() => setCreating(true)}>
            <Plus size={15} />
            {t("users.actions.create")}
          </Button>
        </div>
        <form
          className="mb-4 flex max-w-md items-center gap-2 rounded-xl border bg-white px-3"
          onSubmit={(event) => {
            event.preventDefault();
            void load();
          }}
        >
          <Search size={15} className="text-slate-400" />
          <input
            aria-label={t("users.search.label")}
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={t("users.search.placeholder")}
            className="h-10 min-w-0 flex-1 outline-none"
          />
        </form>
        {error && (
          <p
            role="alert"
            className="mb-4 rounded-xl bg-red-50 p-3 text-sm text-red-700"
          >
            {error}
          </p>
        )}
        <div className="overflow-x-auto rounded-2xl border bg-white">
          <table className="w-full text-left text-sm" data-testid="users-table">
            <thead className="border-b bg-slate-50 text-xs uppercase text-slate-500">
              <tr>
                <th className="p-3">{t("users.fields.user")}</th>
                <th className="p-3">{t("users.fields.role")}</th>
                <th className="p-3">{t("users.fields.source")}</th>
                <th className="p-3">{t("users.fields.status")}</th>
                <th className="p-3">{t("users.fields.lastLogin")}</th>
                <th className="p-3 text-right">{t("users.fields.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr>
                  <td className="p-8 text-center text-slate-500" colSpan={6}>
                    {t("users.loading")}
                  </td>
                </tr>
              ) : (
                items.map((item) => (
                  <tr key={item.id} className="border-b last:border-0">
                    <td className="p-3">
                      <div className="font-medium">
                        {item.display_name || item.username}
                      </div>
                      <div className="text-xs text-slate-500">
                        {item.username}
                        {item.email ? ` · ${item.email}` : ""}
                      </div>
                    </td>
                    <td className="p-3">{item.role}</td>
                    <td className="p-3">{item.auth_source}</td>
                    <td className="p-3">
                      <span
                        className={`rounded-full px-2 py-1 text-xs font-medium ${statusTone(item.status)}`}
                      >
                        {t(`users.status.${item.status}`)}
                      </span>
                    </td>
                    <td className="p-3 text-slate-500">
                      {item.last_login_at
                        ? new Date(item.last_login_at).toLocaleString()
                        : t("users.never")}
                    </td>
                    <td className="p-3 text-right">
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={!canSoftDeleteUser(item, user.id)}
                        aria-label={t("users.actions.deleteUser", {
                          name: item.username,
                        })}
                        onClick={() => setDeleting(item)}
                      >
                        <Trash2 size={14} />
                      </Button>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </section>
      {creating && (
        <Modal
          title={t("users.create.title")}
          onClose={() => setCreating(false)}
        >
          <form
            className="space-y-3"
            onSubmit={(event) => void create(event)}
            data-testid="create-user-form"
          >
            <Field
              name="username"
              label={t("users.fields.username")}
              required
            />
            <Field
              name="display_name"
              label={t("users.fields.displayName")}
              required
            />
            <Field name="email" label={t("users.fields.email")} type="email" />
            <Field
              name="password"
              label={t("users.fields.password")}
              type="password"
              required
              minLength={6}
            />
            <label className="block text-sm">
              {t("users.fields.role")}
              <select
                name="role"
                className="mt-1 h-10 w-full rounded-lg border px-3"
              >
                <option value="user">user</option>
                <option value="admin">admin</option>
              </select>
            </label>
            <div className="flex justify-end gap-2">
              <Button
                type="button"
                variant="ghost"
                onClick={() => setCreating(false)}
              >
                {t("users.actions.cancel")}
              </Button>
              <Button type="submit">{t("users.actions.create")}</Button>
            </div>
          </form>
        </Modal>
      )}
      {deleting && (
        <Modal
          title={t("users.delete.title")}
          onClose={() => setDeleting(null)}
        >
          <p className="mb-5 text-sm text-slate-600">
            {t("users.delete.message", { name: deleting.username })}
          </p>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setDeleting(null)}>
              {t("users.actions.cancel")}
            </Button>
            <Button variant="destructive" onClick={() => void confirmDelete()}>
              {t("users.actions.confirmDelete")}
            </Button>
          </div>
        </Modal>
      )}
    </main>
  );
}

function Field({
  label,
  ...props
}: React.InputHTMLAttributes<HTMLInputElement> & { label: string }) {
  return (
    <label className="block text-sm">
      {label}
      <input {...props} className="mt-1 h-10 w-full rounded-lg border px-3" />
    </label>
  );
}
function Modal({
  title,
  children,
  onClose,
}: {
  title: string;
  children: React.ReactNode;
  onClose(): void;
}) {
  return (
    <div
      className="fixed inset-0 z-50 grid place-items-center bg-slate-950/40 p-4"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <section
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className="w-full max-w-md rounded-2xl bg-white p-5 shadow-2xl"
      >
        <h2 className="mb-4 text-lg font-semibold">{title}</h2>
        {children}
      </section>
    </div>
  );
}

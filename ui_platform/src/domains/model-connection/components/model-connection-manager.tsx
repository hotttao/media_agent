import { type FormEvent, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  listModelConnections,
  revokeModelConnection,
  saveModelConnection,
} from "@/domains/model-connection/lib/client";
import type { ModelConnectionChoice } from "@/foundation/types/model-connection";

export function ModelConnectionManager() {
  const { t } = useTranslation();
  const [items, setItems] = useState<ModelConnectionChoice[]>([]);
  const [name, setName] = useState("本机 Codex");
  const [auth, setAuth] = useState("");
  const [selected, setSelected] = useState<ModelConnectionChoice>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const file = useRef<HTMLInputElement>(null);
  const load = async () => setItems(await listModelConnections());
  useEffect(() => {
    let active = true;
    listModelConnections()
      .then((v) => {
        if (active) setItems(v);
      })
      .catch((e) => {
        if (active) setError(e.message);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);
  const clear = () => {
    setSelected(undefined);
    setName("本机 Codex");
    setAuth("");
    if (file.current) file.current.value = "";
  };
  async function submit(e: FormEvent) {
    e.preventDefault();
    if (busy || !name.trim() || (!selected && !auth)) return;
    setBusy(true);
    setError("");
    try {
      await saveModelConnection(name.trim(), auth, selected);
      clear();
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("modelConnections.failed"));
    } finally {
      setBusy(false);
    }
  }
  async function revoke(item: ModelConnectionChoice) {
    if (!window.confirm(t("modelConnections.confirmRevoke"))) return;
    setBusy(true);
    try {
      await revokeModelConnection(item);
      if (selected?.id === item.id) clear();
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("modelConnections.failed"));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="settings-model-grid grid gap-6">
      <h2 className="text-xl font-semibold">{t("modelConnections.title")}</h2>
      <div className="settings-model-columns grid gap-6">
        <section>
          <p className="mb-6 text-sm leading-6 text-stone-600">
            {t("modelConnections.description")}
          </p>
          {loading && <p>{t("catalog.loading")}</p>}
          {!loading && !items.length && <p>{t("modelConnections.empty")}</p>}
          <ul className="space-y-3">
            {items.map((item) => (
              <li key={item.id} className="rounded-xl border bg-white p-4">
                <h2 className="font-medium">{item.name}</h2>
                <p className="mt-1 text-xs text-stone-500">
                  Codex · auth.json · v{item.version} ·{" "}
                  {t(
                    item.status === "active"
                      ? "modelConnections.active"
                      : "modelConnections.revoked",
                  )}
                </p>
                {item.status === "active" && (
                  <div className="mt-3 flex gap-2">
                    <Button
                      disabled={busy}
                      variant="outline"
                      onClick={() => {
                        setSelected(item);
                        setName(item.name);
                        setAuth("");
                        if (file.current) file.current.value = "";
                      }}
                    >
                      {t("modelConnections.edit")}
                    </Button>
                    <Button
                      disabled={busy}
                      variant="ghost"
                      onClick={() => void revoke(item)}
                    >
                      {t("modelConnections.revoke")}
                    </Button>
                  </div>
                )}
              </li>
            ))}
          </ul>
        </section>
        <form
          onSubmit={submit}
          className="h-fit space-y-4 rounded-xl border bg-white p-6"
        >
          <h2 className="text-lg font-semibold">
            {t(selected ? "modelConnections.edit" : "modelConnections.import")}
          </h2>
          <label className="block text-sm">
            {t("modelConnections.name")}
            <input
              className="mt-2 w-full rounded-lg border p-2"
              value={name}
              maxLength={160}
              disabled={busy}
              onChange={(e) => setName(e.target.value)}
              required
            />
          </label>
          <label className="block text-sm">
            auth.json
            <input
              ref={file}
              type="file"
              accept=".json,application/json"
              disabled={busy}
              className="mt-2 block w-full text-sm file:mr-3 file:rounded-lg file:border file:border-stone-200 file:bg-stone-50 file:px-3 file:py-2 file:text-xs file:font-medium"
              onChange={async (e) => {
                setAuth("");
                setError("");
                const f = e.target.files?.[0];
                if (!f) return;
                if (f.size > 65536) {
                  setError(t("modelConnections.tooLarge"));
                  e.target.value = "";
                  return;
                }
                try {
                  setAuth(await f.text());
                } catch {
                  setError(t("modelConnections.failed"));
                }
              }}
            />
          </label>
          <p className="text-xs leading-5 text-stone-500">
            {t("modelConnections.writeOnly")}
          </p>
          {error && (
            <p
              role="alert"
              className="rounded-lg bg-red-50 p-3 text-sm text-red-700"
            >
              {error}
            </p>
          )}
          <div className="flex gap-2">
            <Button
              disabled={busy || !name.trim() || (!selected && !auth)}
              type="submit"
            >
              {t(busy ? "modelConnections.saving" : "modelConnections.save")}
            </Button>
            {selected && (
              <Button
                variant="outline"
                type="button"
                onClick={clear}
                disabled={busy}
              >
                {t("modelConnections.cancel")}
              </Button>
            )}
          </div>
        </form>
      </div>
    </div>
  );
}

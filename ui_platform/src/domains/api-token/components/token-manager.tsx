import { type FormEvent, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { newTokenAttemptKey } from "@/domains/api-token/lib/attempt-key";
import {
  createToken,
  listTokens,
  revokeToken,
} from "@/domains/api-token/lib/client";
import type { ApiToken } from "@/domains/api-token/types";

export function TokenManager() {
  const { t } = useTranslation();
  const [items, setItems] = useState<ApiToken[]>([]);
  const [name, setName] = useState("");
  const [expires, setExpires] = useState("");
  const [mcp, setMcp] = useState(true);
  const [api, setApi] = useState(false);
  const [secret, setSecret] = useState("");
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [attempted, setAttempted] = useState(false);
  const attempt = useRef(newTokenAttemptKey());
  const mounted = useRef(true);
  const inFlight = useRef(false);
  async function load() {
    setLoading(true);
    try {
      const rows = await listTokens();
      if (mounted.current) setItems(rows);
    } catch {
      if (mounted.current)
        setError((current) => current || t("apiTokens.failed"));
    } finally {
      if (mounted.current) setLoading(false);
    }
  }
  useEffect(() => {
    mounted.current = true;
    let active = true;
    listTokens()
      .then((rows) => {
        if (active) setItems(rows);
      })
      .catch(() => {
        if (active) setError(t("apiTokens.failed"));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
      mounted.current = false;
    };
  }, [t]);
  function reset() {
    setSecret("");
    setName("");
    setExpires("");
    setAttempted(false);
    setError("");
    attempt.current = newTokenAttemptKey();
  }
  async function submit(e: FormEvent) {
    e.preventDefault();
    if (inFlight.current || attempted || !name.trim() || (!mcp && !api)) return;
    const date = new Date(expires);
    if (!Number.isFinite(date.getTime()) || date.getTime() <= Date.now()) {
      setError(t("apiTokens.futureExpiry"));
      return;
    }
    inFlight.current = true;
    setBusy(true);
    setAttempted(true);
    setSecret("");
    setError("");
    try {
      const receipt = await createToken({
        name: name.trim(),
        expires_at: date.toISOString(),
        scopes: [
          ...(mcp ? ["mcp.access"] : []),
          ...(api ? ["api.access"] : []),
        ],
        idempotency_key: attempt.current,
      });
      if (mounted.current) {
        setSecret(receipt.secret);
        if (!receipt.secret) setError(t("apiTokens.unknown"));
      }
    } catch {
      if (mounted.current) setError(t("apiTokens.unknown"));
    } finally {
      inFlight.current = false;
      if (mounted.current) {
        setBusy(false);
        await load();
      }
    }
  }
  async function revoke(id: string) {
    if (inFlight.current || !window.confirm(t("apiTokens.confirmRevoke")))
      return;
    inFlight.current = true;
    setBusy(true);
    setError("");
    try {
      await revokeToken(id);
      if (mounted.current) {
        setSecret("");
        await load();
      }
    } catch {
      if (mounted.current) setError(t("apiTokens.failed"));
    } finally {
      inFlight.current = false;
      if (mounted.current) setBusy(false);
    }
  }
  return (
    <section className="space-y-5">
      <h2 className="text-xl font-semibold">{t("apiTokens.title")}</h2>
      <p className="text-sm text-stone-600">{t("apiTokens.description")}</p>
      {error && (
        <p role="alert" className="text-sm text-red-700">
          {error}
        </p>
      )}
      {secret && (
        <div className="space-y-3 rounded-xl border border-amber-300 p-4">
          <p>{t("apiTokens.once")}</p>
          <code className="block break-all" data-testid="token-secret">
            {secret}
          </code>
          <div className="flex gap-2">
            <Button
              type="button"
              onClick={() => {
                if (!navigator.clipboard?.writeText) {
                  setError(t("apiTokens.copyFailed"));
                  return;
                }
                navigator.clipboard
                  .writeText(secret)
                  .catch(() => setError(t("apiTokens.copyFailed")));
              }}
            >
              {t("apiTokens.copy")}
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={() => setSecret("")}
            >
              {t("apiTokens.close")}
            </Button>
          </div>
        </div>
      )}
      <form onSubmit={submit} className="space-y-4 rounded-xl border p-4">
        <label className="block space-y-1">
          <span>{t("apiTokens.name")}</span>
          <input
            required
            maxLength={120}
            disabled={busy || attempted}
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="block w-full rounded border p-2"
          />
        </label>
        <label className="block space-y-1">
          <span>{t("apiTokens.expires")}</span>
          <input
            required
            type="datetime-local"
            disabled={busy || attempted}
            value={expires}
            onChange={(e) => setExpires(e.target.value)}
            className="block w-full rounded border p-2"
          />
        </label>
        <fieldset disabled={busy || attempted} className="space-y-2">
          <legend>{t("apiTokens.scopes")}</legend>
          <label className="flex items-center gap-2">
            <input
              type="checkbox"
              checked={mcp}
              onChange={(e) => setMcp(e.target.checked)}
            />
            {t("apiTokens.mcp")}
          </label>
          <label className="flex items-center gap-2">
            <input
              type="checkbox"
              checked={api}
              onChange={(e) => setApi(e.target.checked)}
            />
            {t("apiTokens.api")}
          </label>
        </fieldset>
        <div className="flex flex-wrap gap-2">
          <Button type="submit" disabled={busy || attempted || (!mcp && !api)}>
            {t("apiTokens.create")}
          </Button>
          <Button
            type="button"
            variant="outline"
            disabled={busy}
            onClick={reset}
          >
            {t("apiTokens.newAttempt")}
          </Button>
        </div>
      </form>
      <Button
        type="button"
        variant="outline"
        disabled={loading || busy}
        onClick={() => {
          setError("");
          void load();
        }}
      >
        {t("apiTokens.refresh")}
      </Button>
      {loading ? (
        <p role="status">{t("apiTokens.loading")}</p>
      ) : !items.length ? (
        <p>{t("apiTokens.empty")}</p>
      ) : null}
      <ul className="space-y-3">
        {items.map((item) => (
          <li key={item.id} className="space-y-2 rounded-xl border p-4">
            <h3 className="break-words font-medium">{item.name}</h3>
            <p className="break-all text-sm">{item.scopes?.join(", ")}</p>
            <p className="text-sm">
              {item.status} · {item.expires_at}
            </p>
            {item.status !== "revoked" && (
              <Button
                variant="outline"
                disabled={busy}
                onClick={() => void revoke(item.id)}
              >
                {t("apiTokens.revoke")}
              </Button>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

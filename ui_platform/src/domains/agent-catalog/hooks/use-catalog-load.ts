import { useCallback, useEffect, useState } from "react";
export function useCatalogLoad<T>(load: () => Promise<T>) {
  const [data, setData] = useState<T>();
  const [error, setError] = useState<unknown>();
  const [loading, setLoading] = useState(true);
  const [revision, setRevision] = useState(0);
  const reload = useCallback(() => setRevision((value) => value + 1), []);
  // biome-ignore lint/correctness/useExhaustiveDependencies: revision intentionally reloads after a mutation or retry.
  useEffect(() => {
    let active = true;
    setLoading(true);
    setError(undefined);
    setData(undefined);
    void load()
      .then((value) => {
        if (active) setData(value);
      })
      .catch((cause: unknown) => {
        if (active) setError(cause);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [load, revision]);
  return { data, error, loading, reload };
}

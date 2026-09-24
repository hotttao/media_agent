import { ImageIcon } from "lucide-react";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { cn } from "@/foundation/lib/utils";

export function libraryObjectResolveURL(
  organizationId: string | undefined,
  objectPath: string | undefined,
): string | undefined {
  if (!organizationId || !objectPath) return undefined;
  const query = new URLSearchParams({
    organization_id: organizationId,
    object_path: objectPath,
  });
  return `/api/v1/library-objects:resolve?${query.toString()}`;
}

export function LibraryImage({
  organizationId,
  objectPath,
  alt,
  className,
  fallbackClassName,
}: {
  organizationId?: string;
  objectPath?: string;
  alt: string;
  className?: string;
  fallbackClassName?: string;
}) {
  const [src, setSrc] = useState<string>();
  const [failed, setFailed] = useState(false);
  const resolveURL = libraryObjectResolveURL(organizationId, objectPath);

  useEffect(() => {
    setSrc(undefined);
    setFailed(false);
    if (!resolveURL) return;

    const controller = new AbortController();
    void fetch(resolveURL, {
      credentials: "include",
      signal: controller.signal,
    })
      .then(async (response) => {
        if (!response.ok) throw new Error(`resolve image: ${response.status}`);
        return (await response.json()) as { url?: string };
      })
      .then((result) => {
        if (!result.url) throw new Error("resolved image URL is empty");
        setSrc(result.url);
      })
      .catch((error: unknown) => {
        if (error instanceof DOMException && error.name === "AbortError")
          return;
        setFailed(true);
      });
    return () => controller.abort();
  }, [resolveURL]);

  if (!resolveURL || failed) {
    return (
      <div
        className={cn(
          "flex h-full w-full items-center justify-center bg-foreground/[0.035]",
          fallbackClassName,
        )}
      >
        <ImageIcon className="h-9 w-9 text-muted-foreground/60" />
      </div>
    );
  }

  if (!src) {
    return (
      <div
        className={cn(
          "h-full w-full animate-pulse bg-foreground/[0.05]",
          fallbackClassName,
        )}
      />
    );
  }

  return (
    <img
      src={src}
      alt={alt}
      className={cn("h-full w-full object-cover", className)}
      loading="lazy"
      onError={() => setFailed(true)}
    />
  );
}

export function LibraryVideo({
  organizationId,
  objectPath,
  className,
  controls = true,
}: {
  organizationId?: string;
  objectPath?: string;
  className?: string;
  controls?: boolean;
}) {
  const { t } = useTranslation();
  const [src, setSrc] = useState<string>();
  const [failed, setFailed] = useState(false);
  const resolveURL = libraryObjectResolveURL(organizationId, objectPath);
  useEffect(() => {
    setSrc(undefined);
    setFailed(false);
    if (!resolveURL) return;
    const controller = new AbortController();
    void fetch(resolveURL, {
      credentials: "include",
      signal: controller.signal,
    })
      .then((response) => {
        if (!response.ok) throw new Error(`resolve video: ${response.status}`);
        return response.json() as Promise<{ url?: string }>;
      })
      .then((result) => {
        if (!result.url) throw new Error("resolved video URL is empty");
        setSrc(result.url);
      })
      .catch((error: unknown) => {
        if (error instanceof DOMException && error.name === "AbortError")
          return;
        setFailed(true);
      });
    return () => controller.abort();
  }, [resolveURL]);
  return src && !failed ? (
    // biome-ignore lint/a11y/useMediaCaption: migrated source videos have no caption track to attach.
    <video
      className={cn("h-full w-full object-contain", className)}
      src={src}
      controls={controls}
      preload="metadata"
      onError={() => setFailed(true)}
    />
  ) : (
    <div className="flex h-full items-center justify-center bg-muted/40 text-xs text-muted-foreground">
      {t(failed ? "uploadField.videoFailed" : "uploadField.videoLoading")}
    </div>
  );
}

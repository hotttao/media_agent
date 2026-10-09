import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { resolveLibraryMedia } from "@/foundation/lib/library-object";

export function LibraryMedia({
  organizationId,
  objectPath,
  alt,
  video = false,
}: {
  organizationId?: string;
  objectPath?: string;
  alt: string;
  video?: boolean;
}) {
  const { t } = useTranslation();
  const [failedURL, setFailedURL] = useState<string>();
  const media = useQuery({
    queryKey: ["library-media-v2", organizationId, objectPath],
    enabled: Boolean(organizationId && objectPath),
    queryFn: () => resolveLibraryMedia(organizationId!, objectPath!),
    staleTime: 60_000,
  });
  if (
    media.isError ||
    !objectPath ||
    (failedURL && failedURL === media.data?.url)
  )
    return (
      <div className="grid min-h-36 place-items-center gap-2 rounded-xl bg-muted/40 p-4 text-xs text-muted-foreground">
        <p>{t("execution.mediaError")}</p>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => {
            setFailedURL(undefined);
            void media.refetch();
          }}
        >
          {t("v12.retry")}
        </Button>
      </div>
    );
  if (!media.data)
    return <div className="aspect-[4/3] animate-pulse rounded-xl bg-muted" />;
  if (
    video ||
    media.data.content_type?.startsWith("video/") ||
    /\.(mp4|webm|mov)(?:$|[?#])/i.test(media.data.url)
  )
    return (
      // biome-ignore lint/a11y/useMediaCaption: generated media supplies no caption track.
      <video
        controls
        preload="metadata"
        src={media.data.url}
        aria-label={alt}
        onError={() => setFailedURL(media.data.url)}
        className="max-h-[65vh] w-full rounded-xl bg-black object-contain"
      />
    );
  return (
    <img
      src={media.data.url}
      alt={alt}
      onError={() => setFailedURL(media.data.url)}
      className="aspect-[4/3] max-h-[65vh] w-full rounded-xl bg-muted object-contain"
    />
  );
}

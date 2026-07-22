/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Build target: 'standalone' (default) or 'plugin'. */
  readonly VITE_TARGET?: string;
  /** Dev proxy target for /api (materials/workflow gateway). */
  readonly VITE_DEV_API_TARGET?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

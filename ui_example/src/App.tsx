/**
 * Top-level App shell.
 *
 * Per contributing/architecture.md the app composes L3 pages with L1 foundation
 * providers. This skeleton intentionally ships with zero pages — add routes by
 * creating pages/<resource>/list|create|edit|show.tsx and registering them
 * inside <Routes> below.
 */
export default function App() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <header className="border-b px-6 py-4">
        <h1 className="text-lg font-semibold">@media/ui-example</h1>
        <p className="text-sm text-muted-foreground">
          Skeleton only. See{" "}
          <a
            className="underline"
            href="/contributing/architecture.md"
            rel="noreferrer"
          >
            contributing/architecture.md
          </a>{" "}
          for the L0/L1/L2/L3 layer model.
        </p>
      </header>
      <main className="px-6 py-6">
        {/* Mount pages/<resource> routes here. */}
      </main>
    </div>
  );
}

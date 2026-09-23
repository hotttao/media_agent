# @media/ui-workflow

Materials and workflow UI, published as both a standalone application and a
plugin bundle for the platform shell.

## Local development topology

During development, open the Vite server directly at
`http://192.168.2.41:5173`. Vite proxies API and Ory browser-flow paths to the
Traefik backend gateway at `http://192.168.2.41:18080`:

```text
browser -> Vite :5173 -> Traefik :18080 -> Ory / Go services
```

Traefik does not serve the development frontend. After the standalone UI is
ready for deployment, it is built as static assets and served by Nginx behind
Traefik.

## Architecture

The 4-layer model from [`contributing/architecture.md`](./contributing/architecture.md) is the contract:

```
L0  src/components/ui/     shadcn/ui primitives (empty — add via `npx shadcn add`)
L1  src/foundation/        Shared infra — providers, hooks, lib, types
L2  src/domains/<name>/    Resource-specific logic (one folder per resource)
L3  src/pages/<name>/      Route-level views (list/create/edit/show per resource)
```

Higher layers import lower layers only — enforced by `yarn dep-check` (dependency-cruiser).

## Layer-1 sub-trees under `src/foundation/`

| Folder | Purpose |
| --- | --- |
| `components/` | Layout, form containers, table, shared UI |
| `hooks/` | Workspace-aware, delete-confirmation, column-visibility, … |
| `lib/` | Utils, API client, i18n, constants, validators |
| `providers/` | Auth, data, theme, notification providers |
| `types/` | Metadata, BaseStatus, shared serving types |

## Scripts

```bash
yarn install
yarn dev                 # Vite dev server (LAN accessible at http://<192.168.x.x>:5173)
yarn build               # tsc --noEmit && vite build
yarn lint                # biome lint .
yarn lint:fix            # biome lint --write .
yarn format              # biome format --write .
yarn check               # biome check --write .
yarn test                # vitest run
yarn test:coverage       # vitest run --coverage
yarn dep-check           # dependency-cruiser on src/
yarn knip                # unused-export detector
yarn cpd                 # copy/paste detector (jscpd)
yarn test:e2e            # playwright test
```

> Husky pre-commit runs `dep-check → knip → lint-staged → i18n-tracker → check-i18n-keys`.

## Contributing

Read the guides in [`contributing/`](./contributing/) before opening a PR:

- [`contributing/architecture.md`](./contributing/architecture.md) — layer model & dependency rules
- [`contributing/unit-test.md`](./contributing/unit-test.md) — what to test & how
- [`contributing/e2e.md`](./contributing/e2e.md) — Playwright layout, helpers, locator strategy
- [`contributing/i18n-guide.md`](./contributing/i18n-guide.md) — react-i18next workflow

## Adding the first resource

1. Create `src/domains/<name>/` with `types.ts`, optional `components/`, `hooks/`, `lib/`.
2. Create `src/pages/<name>/` with `list.tsx`, `create.tsx`, `edit.tsx`, `show.tsx`.
3. Wire the routes inside `src/App.tsx`.
4. Add the `ResourcePage` fixture to `e2e/fixtures/base.ts` and start writing `e2e/tests/<name>.spec.ts`.

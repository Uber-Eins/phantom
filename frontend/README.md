# Phantom frontend

React 19, Ant Design 6, TypeScript, and Vite power two embedded bundles:

- `index.html`: the authenticated panel SPA under `/panel`
- `login.html`: login and two-factor authentication

Both bundles are written to `../internal/web/dist/` and embedded in the Go
binary. The panel intentionally has no public subscription bundle or external
API documentation bundle.

## Development

Start the Go panel on port 2053, then run:

```sh
npm install
npm run dev
```

Vite serves on `http://localhost:5173` and proxies panel API and WebSocket
requests to the local Go process.

## Checks

```sh
npm run lint
npm run typecheck
npm test
npm run build
npm run build-storybook
```

Storybook browser tests require Chromium once per machine:

```sh
npx playwright install chromium
```

The frontend does not generate OpenAPI or backend-derived schemas. Zod schemas
under `src/schemas` are the source of truth for API parsing, forms, and Xray
configuration data.

## Layout

```text
src/
├── api/          authenticated API queries, mutations, and WebSocket cache updates
├── components/   shared UI and co-located Storybook stories
├── entries/      standalone login bootstrap
├── hooks/        shared React state
├── i18n/         i18next bootstrap; locale JSON lives under internal/web/translation
├── layouts/      panel shell and sidebar
├── lib/xray/     pure link, form, and Xray configuration helpers
├── pages/        dashboard, inbounds, clients, settings, and Xray pages
├── schemas/      Zod schemas
├── styles/       shared styles
├── test/         Vitest tests and golden fixtures
└── utils/        HTTP, clipboard, formatting, and validation helpers
```

The SPA route manifest is in `src/routes.tsx`. New panel API calls must use the
existing session and CSRF-aware HTTP utility.

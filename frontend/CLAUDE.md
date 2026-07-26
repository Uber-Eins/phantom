# Frontend contributor notes

The frontend ships only the authenticated panel SPA and login/2FA bundle.
Do not add public subscription pages, API-token authentication, remote-node
screens, notification transports, or OpenAPI generation.

- Use TanStack Query for server state and invalidate the shared query keys after
  mutations.
- Use the schemas under `src/schemas`; do not hand-write parallel API types.
- Use React Hook Form for form state and Ant Design for components and layout.
- Keep Xray link and wire-format logic in pure helpers under `src/lib/xray`.
- Add user-visible strings to every locale under `internal/web/translation`.
- Run `npm run lint`, `npm run typecheck`, `npm test`, `npm run build`, and
  `npm run build-storybook` before handing off a change.

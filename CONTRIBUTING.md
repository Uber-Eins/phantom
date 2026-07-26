# Contributing

Phantom targets one administrator and one local Xray process. Changes must not reintroduce remote nodes, public subscription endpoints, external authentication, notification transports, IP banning, or in-container binary updates.

## Toolchain

- Go 1.26.5 with a C compiler for SQLite
- Node.js 24+ and npm 10+
- Podman 6+ or Buildah for the OCI image

Install frontend dependencies with `npm ci --ignore-scripts` in `frontend/`.

## Verification

```sh
make verify
```

The gate runs Go formatting and tests, frontend lint/type checks/tests/build, the Storybook compile check, and Quadlet validation.

Keep files focused by responsibility. Add migration tests for persisted-data changes and route tests for removed or changed HTTP surfaces. The local share-link generator must remain independent from any public subscription server.

Build the release-shaped image locally with:

```sh
podman build --network host --platform linux/amd64 -f Containerfile -t phantom:test .
```

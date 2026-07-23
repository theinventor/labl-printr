# Vendored: ZebraPrintLab

This directory is a pruned, lightly patched copy of
[u8array/ZebraPrintLab](https://github.com/u8array/ZebraPrintLab) (MIT licensed
— see `LICENSE` and `THIRD-PARTY-LICENSES.md`), vendored 2026-07-22 from the
then-current `main`.

Changes from upstream:

- **Pruned for web-only use**: `src-tauri/` (desktop shell), `packages/mcp-server/`,
  `tests/`, `docs/`, `scripts/`, and unit tests removed; `package.json` build
  script simplified accordingly.
- **`vite.config.ts`**: `base` set to `/designer/` (served under that prefix by
  the labl-printr server); Rust license-notice emission dropped with `src-tauri/`.
- **`src/components/Output/ZPLOutput.tsx`**: added a "Send to labl-printr"
  action that POSTs the current label to the host app's `/api/designer-import`.

To pull a newer upstream: re-clone, re-apply the three changes above, rebuild
(`pnpm install && pnpm run build`), and copy `dist/` to
`internal/server/designerdist/`.

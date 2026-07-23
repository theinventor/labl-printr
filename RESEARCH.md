# Research: existing landscape

Survey of prior art before writing code. Three passes: printing/architecture patterns, ZPL libraries, and existing projects worth borrowing from.

## How to talk to the printer (and be honest about it)

The ZD421 is a Link-OS printer. The print path is simple — open TCP to port **9100**, write ZPL, close — but 9100 is a raw byte sink with **no acknowledgment protocol**: the printer's TCP stack accepts and buffers jobs even when it's paused, out of media, or has the head open. Naive fire-and-forget reports "success" for labels that never printed, and buffered jobs can vanish on power-cycle.

Zebra's own [best-practices doc](https://techdocs.zebra.com/link-os/2-13/bestpractices/content/index.html) prescribes the honest loop:

1. **Pre-check** with `~HS` host status (paper-out flag, pause flag, head-up flag). Quirk: in some error states the printer doesn't answer `~HS` at all — a timeout is itself a "not ready" signal.
2. Send the ZPL.
3. **Confirm drain**: poll `~HS` until formats-in-receive-buffer returns to 0; optionally diff the `odometer.total_label_count` SGD var to prove labels hit paper.

Link-OS printers expose a dedicated **status channel on port 9200** (SGD queries, JSON) so status polling never interleaves with print data on 9100. Port map reference: [Zebra KB 000031643](https://supportcommunity.zebra.com/s/article/000031643).

**Discovery:** Zebra printers answer a proprietary UDP broadcast on port **4201** with model, serial, firmware, and IP — the protocol is [reverse-engineered and documented](https://jfr.im/blog/2024/09/zebra-network-discovery-protocol/), implementable in ~30 lines. Best practice: persist printers by **serial**, resolve IP at print time, so DHCP churn never breaks anything. mDNS/SNMP exist as fallbacks but add little for a Zebra-only v1.

**ZD421 notes:** optional Ethernet module, DHCP by default, has a print-server web UI; FEED+CANCEL self-test prints the network config report with its IP. 203 dpi standard (300 dpi variant exists) — confirm at calibration.

## Job model

Surveyed PrintNode's API, Zebra SendFileToPrinter, and CUPS/IPP semantics:

- **CUPS/IPP** is the canonical state machine: `queued → printing → done | failed | canceled`. Submit returns a job id immediately; everything async after that. 40 years of `lp`/`lpstat` muscle memory — mirror it.
- **PrintNode**'s killer detail: an `Idempotency-Key` header on job submission (duplicate within 24h → 409). Exists specifically to prevent the one scary failure in this domain: **double-printing on network retry**.
- **zpl-rest** (prior art at exactly our scale) shows that job **history + reprint** is the feature people actually use, more than a live queue.
- At home scale the "queue" is a single worker per printer — a mutex with history. Store the **rendered ZPL on the job row**: free `reprint <id>`, free exact-bytes re-preview.

## Preview pipeline

- **[Labelary](https://labelary.com/service.html)** is the de-facto ZPL→PNG/PDF renderer. Free tier: 3 req/s, 5,000/day — ample. ZD421 at 203 dpi = **8 dpmm**. `X-Linter: On` header returns ZPL warnings — free lint for a `check` command.
- The golden rule from every project that gets preview right: **preview the exact post-template ZPL bytes** at the printer's true dpmm/dimensions. Never maintain a parallel preview template — there is no driver layer in ZPL land; the artifact sent to the printer *is* the string.
- Local/offline renderer options if the cloud dependency chafes: [zebrash](https://github.com/ingridhq/zebrash) (Go, MIT, subset of ZPL), BinaryKits.Zpl (C#).
- **Dev loop without hardware**: [Virtual-ZPL-Printer](https://github.com/porrey/Virtual-ZPL-Printer) listens on TCP 9100 and renders incoming ZPL — a fake ZD421 while the real one is offline.

## ZPL generation libraries

### Node / TypeScript

| Library | License | Maintained? | What it is | Verdict |
|---|---|---|---|---|
| [jszpl](https://github.com/DanieLeeuwner/JSZPL) | **GPL-3.0** (verified on npm) | Yes — v2 TS rewrite, active 2026 | Real layout engine: grids, boxes, alignment, nesting, QR + many barcodes | Best JS layout engine, but GPL would force labl-printr to GPL |
| [zpl-image](https://github.com/metafloor/zpl-image) | MIT | Yes | PNG/JPEG/GIF → `^GFA` bitmap with Z64 compression, node + browser | **Use** for image→ZPL |
| [zpl-js](https://github.com/tomoeste/zpl-js) | MIT | Yes, early (0.1.x) | In-browser ZPL parser + renderer subset, live-preview editor, React hooks | Watch; possible web-UI preview component |
| [zpl-toolchain](https://github.com/trevordcampbell/zpl-toolchain) | MIT/Apache-2.0 | Yes, early | Rust core with TS bindings: full ZPL parser, **linter (46 diagnostics)**, formatter, TCP print client with `~HS` status | Useful as a lint step; no renderer yet |
| [@schie/fluent-zpl](https://github.com/schie/fluent-zpl) | MIT | Yes, tiny | Zero-dep fluent ZPL builder | API inspiration for a hand-rolled builder |
| node-zpl | MIT | Dormant | Thin partial command wrapper | Skip |

### Python

| Library | License | Maintained? | What it is | Verdict |
|---|---|---|---|---|
| [zpl](https://github.com/cod3monk/zpl) (cod3monk) | **AGPL-3.0** | Yes | mm-based label builder, text/graphics/QR, built-in Labelary preview | Best-known, but AGPL |
| [simple-zpl2](https://github.com/sacherjj/simple_zpl2) | MIT | Stale (2020) | Validated command builders + NetworkPrinter class | OK but old |
| [zebrafy](https://github.com/miikanissi/zebrafy) | LGPL-3.0 | Semi | Image/PDF ↔ ZPL both directions, Z64, dithering | Best Python image↔ZPL |

### Offline ZPL renderers (Labelary alternatives)

| Project | License | Status | Notes |
|---|---|---|---|
| [zpl-renderer-js](https://github.com/Fabrizz/zpl-renderer-js) | MIT (verified on npm) | Active, v4.0.0 (2026) | **zebrash compiled to WASM** — browser + Node, offline, no rate limits |
| [labelize](https://github.com/GOODBOY008/labelize) | MIT | Active | Rust ZPL+EPL → PNG/PDF; CLI, HTTP microservice, or library |
| [zebrash](https://github.com/ingridhq/zebrash) | MIT | Works; maintainer stepped away | Go renderer, handles carrier-grade labels, major barcodes |
| [BinaryKits.Zpl.Viewer](https://github.com/BinaryKits/BinaryKits.Zpl) | MIT | Active | C# parser+renderer; the Analyzer/Drawer architecture zebrash is based on |

### Transport options (ranked for a ZD421 on LAN)

1. **Raw TCP 9100** — open socket, write `^XA…^XZ`, close. What every project in the survey uses. ✅
2. LPR/LPD (port 515) — adds nothing here. IPP — only on Link-OS firmware ≥ 7.4. Link-OS SDK — Java/.NET only, no Node/Python. Zebra Browser Print — per-machine agent install, irrelevant when the *server* owns the socket. All skipped.

## ZD421 hardware notes

- Variants: ZD421**D** (direct thermal) / ZD421**T** (transfer) / ZD421**C** (cartridge); **203 dpi or 300 dpi, fixed per unit**. Max print width 4.09".
- **2.4" media math**: 203 dpi → `^PW487` (487 dots); 300 dpi → `^PW720`. Labelary preview = `2.4x<len>` at 8 dpmm (203 dpi).
- **Continuous media**: `^MNN` (no gap sensing) + **`^LL<dots>` required in every format**; `^MNV` = variable length (auto-extends on overflow). Persist `media.type = continuous` via SGD so the printer never gap-hunts; SmartCal can misread continuous stock.
- **Centering quirk**: ZD-series roll holders center narrow media under the 4" head — with 2.4" stock, x=0 lands off-media. Expect a left shift of ≈ (832−487)/2 ≈ **172 dots** via `^LS`, verified with a test print.
- ⚠️ **Base ZD421 SKUs are USB-only** — Ethernet is a modular slot add-on (module P1112640-015, field-installable) or factory Wi-Fi. Confirm the unit has a network module before it comes online.

## Existing projects worth borrowing from

### The blueprint tier

| Project | What it is | What to borrow |
|---|---|---|
| [Syfaro/zpl-printer](https://github.com/Syfaro/zpl-printer) | Rust self-hosted web app for network Zebras | The closest architecture match that exists: printers + label sizes + ZPL templates with variables, Labelary live-preview playground, REST print API, raw-send endpoint, printer alert monitoring |
| [Labelito](https://github.com/chiva/labelito) (GPL-3) | FastAPI print server for Brother printers, very active | The template & API design: declarative **YAML label templates** (`{{var}}` interpolation, QR, barcode, rows/columns, computed dates), `/print` + `/preview` + `/reprint/{job}` + `/printer/status`, media-mismatch detection, hot template reload. Right shape, wrong printer brand — patterns only (GPL) |
| [brother_ql_web](https://github.com/FriedrichFroebel/brother_ql_web) (GPL-3) | The canonical "type text → live preview → print" web UI | UX reference for the Large/Small Print templates on continuous media: font picker, size, alignment, QR+text combo, mobile-responsive. Patterns only (GPL) |
| [ZebraPrintLab](https://github.com/u8array/ZebraPrintLab) (**MIT**, very active) | React 19 + Konva drag-and-drop ZPL label designer with **lossless ZPL import/round-trip**, 28 barcode symbologies via bwip-js, CSV batch, Labelary preview | Could be embedded or forked outright if/when we want a visual designer beyond fixed templates |

### Useful building blocks

- [jszpl](https://github.com/DanieLeeuwner/JSZPL) (MIT, TS) — programmatic ZPL with layout primitives (grid/text/barcode) for a Node backend
- [zebrafy](https://github.com/miikanissi/zebrafy) (LGPL, Python, active) — PNG/PDF→ZPL `^GF` with Z64 compression; cleanest image→ZPL escape hatch
- [metafloor/zpl-image](https://github.com/metafloor/zpl-image) (MIT, JS) — image→GRF/Z64 for JS stacks
- [bwip-js](https://github.com/metafloor/bwip-js) (MIT) — canonical barcode/QR raster generation for browser previews
- [zebra_day](https://github.com/Daylily-Informatics/zebra_day) (GPL) — printer fleet discovery by 9100 scan + template store ideas
- [zpl-rest](https://github.com/mrothenbuecher/zpl-rest) (MIT, stale) — minimal clean data model: stored ZPL with `${var}` placeholders, jobs + reprint
- [InvenTree's label system](https://docs.inventree.org/en/stable/report/) + [inventree-zebra-plugin](https://github.com/SergeoLacruz/inventree-zebra-plugin) — gold standard for the HTML/CSS→PNG→ZPL template pipeline, with `{% qrcode %}` template tags and a live-preview template editor
- [zplbox](https://github.com/ricebean-net/zplbox) (AGPL, active) — HTML/PDF→ZPL microservice; usable as an unmodified Docker sidecar if we ever want HTML templates

### HTML-render pipeline vs native ZPL

Two camps in the wild. **HTML→PNG→`^GF` graphic** (InvenTree, zplbox): any font/layout, preview trivially identical to print — but needs a render dependency, 1-bit dither tuning, bigger payloads, and raster barcodes must hit exact module sizes at 203 dpi to scan. **Native ZPL templates with placeholder substitution** (Syfaro, zpl-rest, PrintZPL): crisp native `^BQ` QR and `^BC` barcodes, tiny payloads, `^LL` handles continuous length trivially — but one built-in scalable font and layout-by-preview-iteration.

**Consensus hybrid from the prior art:** our four planned templates are all achievable in clean native ZPL; keep an image→ZPL escape hatch for logos/arbitrary images later.

### QR content schemes (inventory prior art)

- **Snipe-IT**: QR = full URL to the asset page; often paired with a second 1D barcode of the asset tag for USB scanners (dual-code pattern)
- **InvenTree**: compact alphanumeric short-codes (`INV-…`) — uppercase/alphanumeric-only payloads make dramatically smaller, faster-scanning QRs
- **Homebox**: full instance URL `https://host/item/{uuid}`
- **Takeaway for labl-printr**: encode a short canonical URL (`https://host/q/{short-id}` → 302 to the target). Small QR matters at 2.4" @ 203 dpi.

### License cautions

MIT/permissive and safe to borrow code from: ZebraPrintLab, jszpl, zpl-image, bwip-js, zpl-rest, PrintZPL. GPL/AGPL (patterns only, or unmodified sidecar): Labelito, brother_ql_web, zebra_day, zplbox. CC BY-NC (ideas only): Dodoooh/brother_ql_app.

## Recommended architecture (synthesis)

**Node/TypeScript single service** — web UI + REST API in one process, CLI as a thin client of the same API. One code path, one job history. (The JS ecosystem won this on merit: the only actively-maintained offline ZPL renderer with no runtime dependency is a WASM npm package, and the MIT building blocks — `zpl-image`, `bwip-js` — are all JS.)

- **License: MIT** for labl-printr → hand-roll the ZPL builder (~200 lines of string commands; crib the fluent API from BinaryKits.Zpl) instead of depending on GPL jszpl. `zpl-image` (MIT) for the image escape hatch.
- **Templates**: parameterized native ZPL for the four v1 templates (Inventory w/ native `^BQ` QR, Large Print, Small Print, Packing Room/Contents) — declarative template definitions à la Labelito, stored server-side. All four are clean native-ZPL jobs; no HTML pipeline needed in v1.
- **Print path**: raw TCP 9100. Per job: `~HQES` pre-check (refuse on paper-out/pause/head-up; timeout = not ready) → send ZPL → poll `~HS` until formats-in-buffer hits 0 → mark done. Status polling on port 9200 so it never queues behind print data.
- **Job model**: SQLite; `queued → printing → done | failed | canceled`; idempotency key on submit (the one scary bug in this domain is double-printing on retry); rendered ZPL stored on the job row → free `reprint <id>` and exact-bytes re-preview.
- **Preview**: render the exact post-template ZPL through `zpl-renderer-js` (offline, instant) in the web UI; Labelary API as the correctness oracle during development; `X-Linter: On` for free ZPL linting.
- **Discovery**: Zebra UDP-4201 magic packet (~30 lines); persist printers by serial, resolve IP at print time.
- **Dev loop before the printer is online**: built-in "virtual printer" mode — listen on 9100, render whatever arrives. Develop and demo end-to-end with zero hardware.
- **CLI**: `lp`-style async ergonomics — `labl print inventory --var sku=ABC123 --copies 2`, `labl preview … -o out.png`, `labl print - < raw.zpl`, `labl printers|discover|status|jobs|reprint <id>`. Default printer stored in the server DB.

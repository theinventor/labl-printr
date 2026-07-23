# labl-printr

Self-hosted label printing for network thermal printers. Pick a template in the web UI, or fire jobs from the CLI/API — the label comes out of the printer.

## What it does

- **Web interface** — choose a label template, fill in the fields, see a pixel-accurate preview, print.
- **REST API** — `POST /print` with a template name + fields; returns a job you can check on.
- **CLI** — `labl print inventory --sku ABC123 --url https://…` from any machine on the LAN.

## Templates (v1)

| Template | Contents |
|---|---|
| Inventory label | Product name, SKU, product info, QR code pointing at a custom URL |
| Large print | Big bold text, auto-sized to fit |
| Small print | Compact text label |
| Packing label | ROOM in big type / Contents list below |

## Hardware

First supported printer: **Zebra ZD421** (direct thermal), on the LAN.

- Media: 2.4" continuous (variable-length labels — `^LL` set per job)
- At 203 dpi that's a 487-dot print width (`^PW487`); ZD421 also ships in a 300 dpi variant (720 dots) — dpi to be confirmed when the printer comes online
- Transport: ZPL over raw TCP port 9100; status via `~HQES`/`~HS` on the dedicated 9200 status channel
- ⚠️ Base ZD421 SKUs are USB-only — Ethernet/Wi-Fi is a modular add-on; confirm the unit has a network module
- A **virtual printer mode** (listen on 9100 → render to PNG) covers development until the hardware is online

## Status

🚧 Day zero. Research on the existing open-source landscape lives in [RESEARCH.md](RESEARCH.md); architecture takes shape from there.

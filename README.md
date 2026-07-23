# labl-printr

Self-hosted label printing for network Zebra printers. Pick a template in the web UI, fire jobs from the CLI or API — the label comes out of the printer. Runs as one small service on your LAN.

![Go](https://img.shields.io/badge/go-1.26-00ADD8) ![License](https://img.shields.io/badge/license-MIT-green)

## What you get

- **Web UI** — template gallery, live pixel-accurate preview on virtual label paper, one-click print. Dark, fast, pretty.
- **Visual designer** — a vendored [ZebraPrintLab](https://github.com/u8array/ZebraPrintLab) (MIT) served at `/designer/`, with a **Send to labl-printr** button that saves your canvas as a reusable template — variables included.
- **REST API** — `POST /api/jobs` with a template + fields; idempotency keys make retries safe (no double labels).
- **`labl` CLI** — `labl print inventory --var sku=ABC123 …` from any machine on the LAN.
- **Virtual printer** — built in, on by default: everything "printed" renders to PNG in an output tray in the UI, and it listens on TCP 9100 so you can even `netcat` ZPL at it. Develop and demo with zero hardware.
- **Honest job status** — real Zebra printers accept jobs on port 9100 even when out of paper. labl-printr pre-checks `~HQES`, sends, then polls `~HS` until the buffer drains — `done` means the label physically printed.

## Built-in templates (2.4″ continuous media)

| Template | Contents |
|---|---|
| **Inventory** | Product name, SKU, details, QR code → custom URL |
| **Large Print** | Big bold text, auto-sized to fill the width |
| **Small Print** | Compact utility label — cables, jars, shelf edges |
| **Packing** | Inverse ROOM banner + contents list |

Labels are variable-length: an auto-trim pass renders each label, measures where the ink ends, and cuts `^LL` to fit.

## Quick start

```sh
git clone https://github.com/theinventor/labl-printr && cd labl-printr
make server && ./bin/labl-server
# → web UI on http://localhost:5225, virtual printer on tcp/9100
```

The built web UI and designer are committed under `internal/server/`, so a plain Go toolchain is all you need. Rebuild frontends with `make web` / `make designer` (Node 22+).

### CLI

```sh
make cli && cp bin/labl ~/.local/bin/
labl config set-server http://your-host:5225

labl templates
labl print inventory --var name="M3 screws" --var sku=HW-M3-012 --var url=https://inv.local/123
labl print large-print --var text=FRAGILE --copies 3
labl print packing --var room=Kitchen --var contents=$'Pots\nPans'
cat label.zpl | labl raw
labl preview inventory --var … -o label.png     # render without printing
labl print … --dry-run                          # validate + size only
labl jobs · labl reprint 12 · labl status · labl printers · labl discover
```

### Docker

```sh
docker build -t labl-printr .
docker run -d -p 5225:5225 -p 9100:9100 -v labl-data:/home/labl/data labl-printr
```

## Adding your real printer

1. Printer needs a network module (base ZD421s are USB-only — Ethernet and Wi-Fi 802.11ac modules are field-installable).
2. **Printers → Scan network** (Zebra UDP-4201 discovery), or **Add manually** with its IP/hostname.
3. Pick 203 or 300 dpi to match the unit — dot math and the narrow-media centering shift are handled per printer.

## API sketch

```
GET  /api/templates                 GET  /api/printers
POST /api/preview                   POST /api/printers/discover
POST /api/jobs                      GET  /api/printers/{id}/status
GET  /api/jobs?limit=50             POST /api/designer-import
POST /api/jobs/{id}/reprint         GET  /api/tray
GET  /api/jobs/{id}/preview.png     GET  /api/tray/{id}.png
```

Job states: `queued → printing → done | failed`. Pass `idempotencyKey` on `POST /api/jobs` and retries return the existing job instead of printing twice.

## Architecture notes

Single Go binary: chi HTTP server + SQLite (pure-Go driver) + embedded [zebrash](https://github.com/ingridhq/zebrash) ZPL renderer — previews and the virtual printer render the **exact bytes** sent to hardware, so preview == print. The ZPL builder is ~200 lines in `internal/zpl` (kept in-house so the project stays MIT; the fancier layout engines are GPL). Research on the landscape that shaped all this: [RESEARCH.md](RESEARCH.md).

Hardware target: Zebra ZD421 (Link-OS), 2.4″ continuous direct-thermal media, ZPL over raw TCP 9100, status via `~HS`/`~HQES` on the dedicated 9200 channel.

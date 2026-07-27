# Bundled fonts

labl-printr renders label text from these embedded TrueType fonts. All are
redistributable; their license texts sit next to the `.ttf` files in `files/`.

Fine-point-pen faces (thin single-stroke):

| ID | Face | Designer | License |
|---|---|---|---|
| `caveat` | Caveat | Impallari Type | OFL 1.1 |
| `shadows` | Shadows Into Light | Kimberly Geswein | OFL 1.1 |
| `ballpoint` | Ballpoint (Nothing You Could Do) | Kimberly Geswein | OFL 1.1 |
| `reenie` | Reenie Beanie | James Grieshaber | OFL 1.1 |
| `justhand` | Just Another Hand | Kimberly Geswein | Apache-2.0 |
| `kalam-fine` | Kalam Fine (Regular) | Indian Type Foundry | OFL 1.1 |

Marker / bolder faces:

| ID | Face | Designer | License |
|---|---|---|---|
| `marker` | Permanent Marker | Font Diner | Apache-2.0 |
| `patrick` | Patrick Hand | Patrick Wagesreiter | OFL 1.1 |
| `kalam` | Kalam Bold | Indian Type Foundry | OFL 1.1 |
| `gochi` | Gochi Hand | Huerta Tipográfica | OFL 1.1 |
| `indie` | Indie Flower | Kimberly Geswein | OFL 1.1 |
| `architect` | Architects Daughter | Kimberly Geswein | OFL 1.1 |

Default template font: `ballpoint`.

The `system` face is not a bundled font — it maps to the printer's resident
scalable font (native ZPL `^A0`), the original look.

All fonts sourced from [Google Fonts](https://github.com/google/fonts). To add
a face: drop the `.ttf` and its license into `files/`, add an entry to the
`registry` in `fonts.go`, done — it's picked up by the embed and the API.

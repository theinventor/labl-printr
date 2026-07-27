// Package media is the catalog of label stock a printer can have loaded. A
// printer record stores which media is currently in it (set from the web UI
// when you swap paper); the render geometry and each driver's encoding derive
// from that media, instead of being hardcoded per driver.
package media

// Media describes one loaded label stock.
type Media struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"` // printer family: network | brother | dymo
	Dpmm       int    `json:"dpmm"`
	WidthDots  int    `json:"widthDots"`  // print width in dots (reading direction)
	Continuous bool   `json:"continuous"` // continuous tape vs fixed die-cut
	LengthDots int    `json:"lengthDots"` // die-cut fixed length; 0 = continuous
	LeftShift  int    `json:"leftShift"`  // ^LS centering (Zebra narrow media)

	// Driver specifics.
	TwoColor     bool   `json:"twoColor"`               // Brother black/red roll
	CupsPageSize string `json:"cupsPageSize,omitempty"` // DYMO CUPS media name
}

// Catalog is every known media, grouped by the printer kind it fits.
var Catalog = []Media{
	// Zebra / ZPL continuous.
	{ID: "zebra-2.4-203", Name: `2.4" continuous (203 dpi)`, Kind: "network", Dpmm: 8, WidthDots: 487, Continuous: true, LeftShift: 172},
	{ID: "zebra-2.4-300", Name: `2.4" continuous (300 dpi)`, Kind: "network", Dpmm: 12, WidthDots: 720, Continuous: true, LeftShift: 280},

	// Brother QL 62mm continuous.
	{ID: "dk2251", Name: "62mm black/red continuous (DK-2251)", Kind: "brother", Dpmm: 12, WidthDots: 696, Continuous: true, TwoColor: true},
	{ID: "dk2205", Name: "62mm mono continuous (DK-2205)", Kind: "brother", Dpmm: 12, WidthDots: 696, Continuous: true, TwoColor: false},

	// DYMO LabelWriter die-cut. WidthDots is the reading (long) dimension; the
	// label prints landscape and CUPS fits it to the die-cut page size.
	{ID: "dymo-address", Name: `Address 1.1"×3.5" (30252)`, Kind: "dymo", Dpmm: 12, WidthDots: 1050, LengthDots: 337, CupsPageSize: "w81h252"},
	{ID: "dymo-3x1", Name: `3"×1" die-cut`, Kind: "dymo", Dpmm: 12, WidthDots: 900, LengthDots: 300, CupsPageSize: "w72h216"},
	{ID: "dymo-shipping", Name: `Shipping 2.3"×4" (30256)`, Kind: "dymo", Dpmm: 12, WidthDots: 1200, LengthDots: 690, CupsPageSize: "w167h288"},
}

// ForKind returns the media choices valid for a printer family.
func ForKind(kind string) []Media {
	var out []Media
	for _, m := range Catalog {
		if m.Kind == kind {
			out = append(out, m)
		}
	}
	return out
}

// Get looks up a media by id.
func Get(id string) (Media, bool) {
	for _, m := range Catalog {
		if m.ID == id {
			return m, true
		}
	}
	return Media{}, false
}

// DefaultFor returns the default media for a printer family (first in catalog).
func DefaultFor(kind string) Media {
	for _, m := range Catalog {
		if m.Kind == kind {
			return m
		}
	}
	return Media{}
}

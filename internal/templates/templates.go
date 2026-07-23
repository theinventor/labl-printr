// Package templates defines labl-printr's built-in label templates and the
// substitution engine for custom (designer-imported) ZPL templates.
package templates

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/theinventor/labl-printr/internal/zpl"
)

// Profile describes the physical printing target a template renders against.
type Profile struct {
	Dpmm      int `json:"dpmm"`      // 8 = 203dpi, 12 = 300dpi
	WidthDots int `json:"widthDots"` // printable width (487 for 2.4" @ 203dpi)
	LeftShift int `json:"leftShift"` // ^LS offset for centered narrow media
}

// DefaultProfile matches the ZD421 203dpi with 2.4" continuous media.
var DefaultProfile = Profile{Dpmm: 8, WidthDots: 487, LeftShift: 0}

// Field is one user-facing input on a template.
type Field struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Type        string `json:"type"` // text | textarea | url
	Required    bool   `json:"required"`
	Placeholder string `json:"placeholder"`
}

// Rendered is the output of a template render.
type Rendered struct {
	ZPL        string
	LengthDots int
}

// Template is anything that can produce ZPL from user field values.
type Template struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Fields      []Field `json:"fields"`
	Builtin     bool    `json:"builtin"`
	render      func(vars map[string]string, p Profile, copies int) (Rendered, error)
}

// Render produces final ZPL for the given variables and printer profile.
func (t *Template) Render(vars map[string]string, p Profile, copies int) (Rendered, error) {
	for _, f := range t.Fields {
		if f.Required && strings.TrimSpace(vars[f.Key]) == "" {
			return Rendered{}, fmt.Errorf("missing required field %q", f.Key)
		}
	}
	if p.WidthDots <= 0 {
		p = DefaultProfile
	}
	if copies < 1 {
		copies = 1
	}
	return t.render(vars, p, copies)
}

// Builtins returns the four v1 templates.
func Builtins() []*Template {
	return []*Template{inventoryTemplate(), largePrintTemplate(), smallPrintTemplate(), packingTemplate()}
}

// Get returns a builtin by id.
func Get(id string) (*Template, bool) {
	for _, t := range Builtins() {
		if t.ID == id {
			return t, true
		}
	}
	return nil, false
}

const margin = 16 // dots of breathing room on every edge

// ---- Inventory label: name + SKU + info on the left, QR to a URL on the right.

func inventoryTemplate() *Template {
	return &Template{
		ID:          "inventory",
		Name:        "Inventory",
		Description: "Product name, SKU, details, and a QR code linking to a URL.",
		Builtin:     true,
		Fields: []Field{
			{Key: "name", Label: "Product name", Type: "text", Required: true, Placeholder: "M3 socket head screws"},
			{Key: "sku", Label: "SKU", Type: "text", Required: true, Placeholder: "HW-M3-012"},
			{Key: "info", Label: "Details", Type: "textarea", Placeholder: "12mm, black oxide, qty 100"},
			{Key: "url", Label: "QR link URL", Type: "url", Required: true, Placeholder: "https://inventory.example/items/123"},
		},
		render: func(vars map[string]string, p Profile, copies int) (Rendered, error) {
			qrMag := 4
			if len(vars["url"]) > 60 {
				qrMag = 3
			}
			qrSide := qrEstSide(vars["url"], qrMag)
			textW := p.WidthDots - qrSide - margin*3

			nameH := zpl.FitFontHeight(vars["name"], textW, 28, 44)
			nameLines := min(zpl.EstLines(vars["name"], nameH, textW), 3)
			skuH := 30
			infoH := 24
			infoLines := 0
			if strings.TrimSpace(vars["info"]) != "" {
				infoLines = min(zpl.EstLines(vars["info"], infoH, textW), 4)
			}

			y := margin
			textBottom := y + nameLines*(nameH+4) + 10 + skuH + 8
			if infoLines > 0 {
				textBottom += 6 + infoLines*(infoH+4)
			}
			length := max(textBottom, margin*2+qrSide) + margin

			l := zpl.NewLabel(p.WidthDots, length, p.LeftShift)
			l.TextBlock(margin, y, nameH, textW, nameLines, 4, zpl.JustifyLeft, vars["name"])
			y += nameLines*(nameH+4) + 10
			l.Text(margin, y, skuH, "SKU "+vars["sku"])
			y += skuH + 8
			if infoLines > 0 {
				y += 6
				l.TextBlock(margin, y, infoH, textW, infoLines, 4, zpl.JustifyLeft, vars["info"])
			}
			qrX := p.WidthDots - qrSide - margin
			l.QR(qrX, margin, qrMag, vars["url"])
			return Rendered{ZPL: l.End(copies), LengthDots: length}, nil
		},
	}
}

// qrEstSide estimates the rendered pixel size of a model-2 QR at a
// magnification: URLs of typical length land at version 4–6 (33–41 modules).
func qrEstSide(data string, mag int) int {
	modules := 33
	if len(data) > 62 {
		modules = 41
	}
	return modules * mag
}

// ---- Large print: text as big as fits.

func largePrintTemplate() *Template {
	return &Template{
		ID:          "large-print",
		Name:        "Large Print",
		Description: "Big bold text, sized to fill the label width.",
		Builtin:     true,
		Fields: []Field{
			{Key: "text", Label: "Text", Type: "textarea", Required: true, Placeholder: "FRAGILE"},
		},
		render: func(vars map[string]string, p Profile, copies int) (Rendered, error) {
			w := p.WidthDots - margin*2
			text := strings.TrimSpace(vars["text"])
			h := zpl.FitFontHeight(text, w, 40, 150)
			lines := zpl.EstLines(text, h, w)
			lineGap := h / 8
			length := margin*2 + lines*(h+lineGap)
			l := zpl.NewLabel(p.WidthDots, length, p.LeftShift)
			l.TextBlock(margin, margin, h, w, lines, lineGap, zpl.JustifyCenter, text)
			return Rendered{ZPL: l.End(copies), LengthDots: length}, nil
		},
	}
}

// ---- Small print: compact utility label.

func smallPrintTemplate() *Template {
	return &Template{
		ID:          "small-print",
		Name:        "Small Print",
		Description: "Compact text label — cables, jars, shelf edges.",
		Builtin:     true,
		Fields: []Field{
			{Key: "text", Label: "Text", Type: "textarea", Required: true, Placeholder: "HDMI — office TV"},
		},
		render: func(vars map[string]string, p Profile, copies int) (Rendered, error) {
			w := p.WidthDots - margin*2
			text := strings.TrimSpace(vars["text"])
			h := 28
			lines := min(zpl.EstLines(text, h, w), 8)
			length := margin*2 + lines*(h+6)
			l := zpl.NewLabel(p.WidthDots, length, p.LeftShift)
			l.TextBlock(margin, margin, h, w, lines, 6, zpl.JustifyLeft, text)
			return Rendered{ZPL: l.End(copies), LengthDots: length}, nil
		},
	}
}

// ---- Packing label: ROOM banner + contents list.

func packingTemplate() *Template {
	return &Template{
		ID:          "packing",
		Name:        "Packing",
		Description: "Room name banner with a contents list — moving boxes, totes.",
		Builtin:     true,
		Fields: []Field{
			{Key: "room", Label: "Room", Type: "text", Required: true, Placeholder: "KITCHEN"},
			{Key: "contents", Label: "Contents", Type: "textarea", Required: true, Placeholder: "Pots and pans\nCutting boards\nKnife block"},
		},
		render: func(vars map[string]string, p Profile, copies int) (Rendered, error) {
			w := p.WidthDots - margin*2
			room := strings.ToUpper(strings.TrimSpace(vars["room"]))
			roomH := zpl.FitFontHeight(room, w-2*margin, 36, 80)
			barH := roomH + 28

			itemH := 28
			var items []string
			for _, line := range strings.Split(vars["contents"], "\n") {
				if s := strings.TrimSpace(line); s != "" {
					items = append(items, s)
				}
			}
			itemLines := 0
			for _, it := range items {
				itemLines += min(zpl.EstLines(it, itemH, w-30), 2)
			}

			length := margin + barH + 14 + itemLines*(itemH+8) + margin
			l := zpl.NewLabel(p.WidthDots, length, p.LeftShift)
			l.InverseText(margin, margin, w, barH, roomH, room)
			y := margin + barH + 14
			for _, it := range items {
				lines := min(zpl.EstLines(it, itemH, w-30), 2)
				l.Text(margin, y+2, itemH, "-")
				l.TextBlock(margin+30, y, itemH, w-30, lines, 8, zpl.JustifyLeft, it)
				y += lines*(itemH+8)
			}
			return Rendered{ZPL: l.End(copies), LengthDots: length}, nil
		},
	}
}

// ---- Custom ZPL templates (designer imports).
//
// Two variable syntaxes: ${key} text placeholders (hand-written ZPL), and
// native ^FNn^FDdefault^FS slots (what ZebraPrintLab exports).

var placeholderRe = regexp.MustCompile(`\$\{([A-Za-z0-9_.-]+)\}`)
var fnSlotRe = regexp.MustCompile(`\^FN(\d+)\^FD([^^~]*)`)

// PlaceholderKeys extracts the sorted unique ${key} names in a raw ZPL string.
func PlaceholderKeys(rawZPL string) []string {
	seen := map[string]bool{}
	for _, m := range placeholderRe.FindAllStringSubmatch(rawZPL, -1) {
		seen[m[1]] = true
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// CustomTemplate wraps stored raw ZPL as a Template. Length is fixed at
// import time (the designer works on a fixed canvas).
func CustomTemplate(id, name string, rawZPL string, lengthDots int) *Template {
	fields := customFields(rawZPL)
	return &Template{
		ID:          id,
		Name:        name,
		Description: "Custom label from the designer.",
		Fields:      fields,
		Builtin:     false,
		render: func(vars map[string]string, p Profile, copies int) (Rendered, error) {
			// User values pass through zpl.Escape so they can never carry ^/~
			// control characters out of their field (same guard as builtins).
			out := placeholderRe.ReplaceAllStringFunc(rawZPL, func(m string) string {
				key := placeholderRe.FindStringSubmatch(m)[1]
				return zpl.Escape(vars[key])
			})
			// Substitute ^FN slot values, then strip the ^FN tokens: outside a
			// ^DF stored format they suppress the field instead of printing it.
			out = fnSlotRe.ReplaceAllStringFunc(out, func(m string) string {
				sub := fnSlotRe.FindStringSubmatch(m)
				if v, ok := vars["field"+sub[1]]; ok && v != "" {
					return "^FD" + zpl.Escape(v)
				}
				return "^FD" + sub[2]
			})
			if copies > 1 && strings.Contains(out, "^XZ") {
				out = strings.Replace(out, "^XZ", fmt.Sprintf("^PQ%d\n^XZ", copies), 1)
			}
			return Rendered{ZPL: out, LengthDots: lengthDots}, nil
		},
	}
}

// customFields derives user-facing fields from both variable syntaxes. ^FN
// slots become optional fields defaulting to their authored ^FD value.
func customFields(rawZPL string) []Field {
	var fields []Field
	for _, k := range PlaceholderKeys(rawZPL) {
		fields = append(fields, Field{Key: k, Label: k, Type: "text", Required: true})
	}
	seen := map[string]bool{}
	for _, m := range fnSlotRe.FindAllStringSubmatch(rawZPL, -1) {
		key := "field" + m[1]
		if seen[key] {
			continue
		}
		seen[key] = true
		fields = append(fields, Field{Key: key, Label: "Field " + m[1], Type: "text", Placeholder: strings.TrimSpace(m[2])})
	}
	return fields
}

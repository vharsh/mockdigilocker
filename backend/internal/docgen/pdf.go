// Package docgen renders synthetic documents as PDF and DigiLocker-style
// certificate XML. Every output carries the demo watermark.
package docgen

import (
	"bytes"
	"fmt"
	"math"
	"strings"
)

const Watermark = "DEMO DOCUMENT – NOT VALID FOR IDENTIFICATION"

// pdfPage is a minimal single-page PDF builder using the standard Helvetica
// fonts (no embedding required), filled rectangles and rotated text.
type pdfPage struct {
	buf  bytes.Buffer
	W, H float64
}

func newPage() *pdfPage { return &pdfPage{W: 595, H: 842} }

// winAnsi converts UTF-8 text to a PDF literal string in WinAnsiEncoding.
func winAnsi(s string) string {
	var b strings.Builder
	b.WriteByte('(')
	for _, r := range s {
		var c byte
		switch {
		case r == '–':
			c = 0x96
		case r == '—':
			c = 0x97
		case r == '•':
			c = 0x95
		case r == '‘', r == '’':
			c = '\''
		case r == '“', r == '”':
			c = '"'
		case r < 256:
			c = byte(r)
		default:
			c = '?'
		}
		switch c {
		case '(', ')', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			if c < 32 {
				continue
			}
			b.WriteByte(c)
		}
	}
	b.WriteByte(')')
	return b.String()
}

func (p *pdfPage) rgb(r, g, b float64) string { return fmt.Sprintf("%.3f %.3f %.3f", r, g, b) }

func (p *pdfPage) Rect(x, y, w, h float64, r, g, b float64) {
	fmt.Fprintf(&p.buf, "%s rg %.2f %.2f %.2f %.2f re f\n", p.rgb(r, g, b), x, y, w, h)
}

func (p *pdfPage) StrokeRect(x, y, w, h, lw float64, r, g, b float64) {
	fmt.Fprintf(&p.buf, "%s RG %.2f w %.2f %.2f %.2f %.2f re S\n", p.rgb(r, g, b), lw, x, y, w, h)
}

func (p *pdfPage) Line(x1, y1, x2, y2, lw float64, r, g, b float64) {
	fmt.Fprintf(&p.buf, "%s RG %.2f w %.2f %.2f m %.2f %.2f l S\n", p.rgb(r, g, b), lw, x1, y1, x2, y2)
}

// Text draws text; font is "F1" (regular) or "F2" (bold).
func (p *pdfPage) Text(x, y float64, font string, size float64, r, g, b float64, s string) {
	fmt.Fprintf(&p.buf, "BT %s rg /%s %.1f Tf %.2f %.2f Td %s Tj ET\n", p.rgb(r, g, b), font, size, x, y, winAnsi(s))
}

// RotText draws text rotated by deg degrees with the given opacity GS.
func (p *pdfPage) RotText(x, y, deg float64, font string, size float64, r, g, b float64, s string) {
	rad := deg * math.Pi / 180
	c, sn := math.Cos(rad), math.Sin(rad)
	fmt.Fprintf(&p.buf, "q /GS1 gs BT %s rg /%s %.1f Tf %.4f %.4f %.4f %.4f %.2f %.2f Tm %s Tj ET Q\n",
		p.rgb(r, g, b), font, size, c, sn, -sn, c, x, y, winAnsi(s))
}

// TextWidth approximates Helvetica text width (good enough for centring).
func TextWidth(s string, size float64, bold bool) float64 {
	w := 0.0
	for _, r := range s {
		switch {
		case r == ' ':
			w += 0.278
		case strings.ContainsRune("il.,:;'|!", r):
			w += 0.26
		case strings.ContainsRune("MWmw@", r):
			w += 0.85
		case r >= 'A' && r <= 'Z':
			w += 0.68
		case r >= '0' && r <= '9':
			w += 0.556
		default:
			w += 0.52
		}
	}
	if bold {
		w *= 1.06
	}
	return w * size
}

// Render serialises the page to a complete PDF file.
func (p *pdfPage) Render(title string) []byte {
	content := p.buf.Bytes()
	var out bytes.Buffer
	offsets := []int{}
	obj := func(body string) {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", len(offsets), body)
	}
	out.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	obj("<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.0f %.0f] /Contents 4 0 R /Resources << /Font << /F1 5 0 R /F2 6 0 R >> /ExtGState << /GS1 7 0 R >> >> >>", p.W, p.H))
	offsets = append(offsets, out.Len())
	fmt.Fprintf(&out, "4 0 obj\n<< /Length %d >>\nstream\n", len(content))
	out.Write(content)
	out.WriteString("\nendstream\nendobj\n")
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
	obj("<< /Type /ExtGState /ca 0.22 /CA 0.22 >>")
	obj(fmt.Sprintf("<< /Title %s /Producer (Mock DigiLocker Sandbox) /Subject %s /Creator (mock-digilocker) >>", winAnsi(title), winAnsi(Watermark)))
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, o := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R /Info %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, len(offsets), xref)
	return out.Bytes()
}

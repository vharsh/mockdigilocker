package docgen

import (
	"bytes"
	"encoding/xml"
	"testing"

	"mockdigilocker/internal/catalog"
	"mockdigilocker/internal/model"
	"mockdigilocker/internal/seed"
)

func TestRenderAllPersonaDocs(t *testing.T) {
	for _, p := range seed.Personas() {
		u := p.User
		for _, ds := range p.Docs {
			d, err := catalog.Build(&u, ds.DocType, ds.Opts)
			if err != nil {
				t.Fatal(err)
			}
			pdf := RenderPDF(d, &u, "Mock Issuer")
			if !bytes.HasPrefix(pdf, []byte("%PDF-1.4")) || !bytes.Contains(pdf, []byte("%%EOF")) || !bytes.Contains(pdf, []byte("NOT VALID FOR IDENTIFICATION")) {
				t.Fatalf("bad pdf for %s", d.URI)
			}
			x := RenderXML(d, &u, "Mock Issuer")
			if err := xml.Unmarshal(x, new(struct{ XMLName xml.Name })); err != nil {
				t.Fatalf("invalid xml for %s: %v", d.URI, err)
			}
		}
		if err := xml.Unmarshal(RenderEAadhaarXML(&u), new(struct{ XMLName xml.Name })); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEffectiveStatus(t *testing.T) {
	d := model.Document{Status: model.DocValid, ValidTo: "2001-01-01"}
	if RenderXML(&d, &model.User{ID: "x"}, "i") == nil {
		t.Fatal()
	}
	if !bytes.Contains(RenderXML(&d, &model.User{ID: "x"}, "i"), []byte(`status="E"`)) {
		t.Fatal("expired doc should have status E")
	}
}

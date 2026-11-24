package scenario

import (
	"testing"
	"time"
)

func TestSelectPrecedence(t *testing.T) {
	now := time.Now()
	all := []Scenario{
		{ID: "g", Enabled: true, Scope: ScopeGlobal, Endpoint: EPAny, Effect: "unexpected_error", CreatedAt: now},
		{ID: "u", Enabled: true, Scope: ScopeUser, TargetID: "u1", Endpoint: EPFile, Effect: "document_not_found", CreatedAt: now},
		{ID: "c", Enabled: true, Scope: ScopeClient, TargetID: "c1", Endpoint: EPFile, Effect: "service_down", CreatedAt: now},
		{ID: "off", Enabled: false, Scope: ScopeClient, TargetID: "c1", Endpoint: EPFile, Effect: "latency", CreatedAt: now.Add(time.Hour)},
	}
	cases := []struct {
		ctx  Ctx
		want string
	}{
		{Ctx{Endpoint: EPFile, ClientID: "c1", UserID: "u1"}, "c"},
		{Ctx{Endpoint: EPFile, ClientID: "c2", UserID: "u1"}, "u"},
		{Ctx{Endpoint: EPFile, ClientID: "c2", UserID: "u2"}, "g"},
		{Ctx{Endpoint: EPLogin}, ""}, // wildcard does not apply to login
	}
	for _, c := range cases {
		got := Select(all, c.ctx)
		id := ""
		if got != nil {
			id = got.ID
		}
		if id != c.want {
			t.Errorf("%+v: got %q want %q", c.ctx, id, c.want)
		}
	}
}

func TestMaxHitsAndProbability(t *testing.T) {
	s := []Scenario{{ID: "x", Enabled: true, Scope: ScopeGlobal, Endpoint: EPToken, Effect: "invalid_client", MaxHits: 1, Hits: 1}}
	if Select(s, Ctx{Endpoint: EPToken}) != nil {
		t.Error("exhausted scenario must not match")
	}
	s[0].MaxHits, s[0].Probability = 0, 0.0001
	hits := 0
	for i := 0; i < 1000; i++ {
		if Select(s, Ctx{Endpoint: EPToken}) != nil {
			hits++
		}
	}
	if hits > 20 {
		t.Errorf("probability ignored: %d hits", hits)
	}
}

func TestCatalogIsConsistent(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range Catalog {
		if seen[e.Key] || len(e.Endpoints) == 0 || e.Label == "" {
			t.Errorf("bad catalog entry %+v", e)
		}
		seen[e.Key] = true
	}
	for _, p := range Presets() {
		for _, s := range p.Scenarios {
			if FindEffect(s.Effect) == nil {
				t.Errorf("preset %s uses unknown effect %s", p.Key, s.Effect)
			}
		}
	}
}

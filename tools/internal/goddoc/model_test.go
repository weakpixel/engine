package goddoc

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func loadSample(t *testing.T) *document {
	t.Helper()
	doc, err := loadDocument("Sample", []string{filepath.Join("testdata", "sample.god.json")})
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestLoadDocument(t *testing.T) {
	doc := loadSample(t)

	var groups []string
	for _, g := range doc.Groups {
		for _, o := range g.Objects {
			groups = append(groups, o.key())
		}
	}
	if want := []string{"ext:Thing", "game:Boss", "game:Hero"}; !slices.Equal(groups, want) {
		t.Errorf("objects = %v, want %v", groups, want)
	}

	var comps []string
	for _, c := range doc.Components {
		comps = append(comps, c.Key)
	}
	// Transform exists in two groups: the keys tell them apart. game:Score
	// is used by no object and still listed.
	want := []string{"ext:Stamp", "ext:Transform", "game:Companion", "game:Health", "game:Score", "game:Transform", "game:Weapon"}
	if !slices.Equal(comps, want) {
		t.Errorf("components = %v, want %v", comps, want)
	}
}

func TestSharedComponentAndEmbedded(t *testing.T) {
	doc := loadSample(t)
	var shared *component
	for _, c := range doc.Components {
		if c.Key == "game:Transform" {
			shared = c
		}
	}
	if shared == nil {
		t.Fatal("shared Transform not found")
	}
	// Hero and Boss share one entry, each recording how it reaches them.
	if len(shared.Users) != 2 {
		t.Fatalf("users = %d, want 2", len(shared.Users))
	}
	boss, hero := shared.Users[0], shared.Users[1]
	if boss.Object.Class != "Boss" || !slices.Equal(boss.Via, []string{"Common"}) {
		t.Errorf("boss usage = %+v", boss)
	}
	if hero.Object.Class != "Hero" || !slices.Equal(hero.Via, []string{"Common", "Base"}) {
		t.Errorf("hero usage = %+v", hero)
	}

	for _, g := range doc.Groups {
		for _, o := range g.Objects {
			for _, c := range o.Components {
				if want := o.Class != "Thing" && c.Field == "Transform"; c.Embedded() != want {
					t.Errorf("%s.%s Embedded() = %v", o.key(), c.Field, c.Embedded())
				}
			}
		}
	}
}

func TestFieldRows(t *testing.T) {
	doc := loadSample(t)
	var comp *component
	for _, c := range doc.Components {
		if c.Key == "game:Companion" {
			comp = c
		}
	}
	var got []string
	for _, r := range comp.Rows {
		got = append(got, strings.Join([]string{r.Name, r.Type, boolStr(r.Required), boolStr(r.Nullable)}, "/"))
	}
	want := []string{
		"Attachment/object/false/true",
		"Attachment.Name/string/true/false",
		"Name/string/true/false",
		"Owner/reference to another object/false/true",
		"Tags/array of string/false/true",
	}
	if !slices.Equal(got, want) {
		t.Errorf("rows = %v, want %v", got, want)
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func TestLoadDocumentErrors(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	comp := func(schemaType string) string {
		return `{"group":"g","name":"C","goType":"x.C","schema":{"type":"` + schemaType + `"}}`
	}
	badJSON := write("bad.json", "{")
	oldVersion := write("old.json", `{"version":3,"objects":[]}`)
	first := write("a.json", `{"version":0,"components":[`+comp("object")+`],"objects":[{"group":"g","class":"o","components":[{"component":"g:C","field":"C"}]},{"group":"g","class":"t","goType":"x.T","components":[]}]}`)
	second := write("b.json", `{"version":0,"components":[`+comp("string")+`],"objects":[{"group":"g","class":"o","components":[{"component":"g:C","field":"Other"}]},{"group":"g","class":"t","goType":"y.T","components":[]}]}`)
	unknown := write("c.json", `{"version":0,"components":[],"objects":[{"group":"g","class":"u","components":[{"component":"g:Nope","field":"N"}]}]}`)
	missing := filepath.Join(dir, "missing.json")

	_, err := loadDocument("t", []string{badJSON, oldVersion, first, second, unknown, missing})
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{
		"bad.json", "unsupported version 3", "regenerate", "missing.json",
		second + ": component g:C differs from the one in " + first,
		second + ": object g:o differs from the one in " + first,
		second + ": object g:t differs from the one in " + first, // goType only
		unknown + ": object g:u uses component g:Nope, which no input defines",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q:\n%v", want, err)
		}
	}
}

func TestLoadDocumentDeterministic(t *testing.T) {
	a, b := loadSample(t), loadSample(t)
	var ta, tb []string
	for _, c := range a.Components {
		ta = append(ta, c.Key)
	}
	for _, c := range b.Components {
		tb = append(tb, c.Key)
	}
	if !slices.Equal(ta, tb) {
		t.Fatalf("%v != %v", ta, tb)
	}
}

func TestTypeString(t *testing.T) {
	doc := loadSample(t)
	for _, c := range doc.Components {
		if c.Key == "ext:Stamp" && c.Type != "time.Time" {
			t.Errorf("Stamp type = %q", c.Type)
		}
	}
}

// TestLoadDocumentMerge checks that identical definitions in several files
// collapse (docs ignored) and that the merged GOD lists each once.
func TestLoadDocumentMerge(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	comp := `{"group":"g","name":"C","goType":"x.C","schema":{"type":"object"}}`
	a := write("a.god.json", `{"version":0,"package":"x/a","components":[`+comp+`],"objects":[{"group":"g","class":"o","components":[{"component":"g:C","field":"C"}]}]}`)
	b := write("b.god.json", `{"version":0,"package":"x/b","components":[{"group":"g","name":"C","doc":"Other docs.","goType":"x.C","schema":{"type":"object","description":"d"}}],"objects":[{"group":"g","class":"o","doc":"Docs.","components":[{"component":"g:C","field":"C","doc":"x"}]},{"group":"g","class":"p","components":[{"component":"g:C","field":"C"}]}]}`)
	doc, err := loadDocument("t", []string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	m := doc.merged()
	if len(m.Components) != 1 || len(m.Objects) != 2 || m.Generator != generatorName || m.Version != 0 {
		t.Fatalf("merged = %+v", m)
	}
	if users := doc.Components[0].Users; len(users) != 2 {
		t.Fatalf("g:C users = %d, want 2", len(users))
	}
}

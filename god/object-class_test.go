package god

import (
	"slices"
	"strings"
	"testing"

	"github.com/weakpixel/ecs"
)

type ocPos struct{ X, Y int }
type ocHealth struct{ HP int }
type ocWeapon struct{ Damage int }
type ocCrown struct{ Gold int }
type ocLoot struct{ Coins int }

var (
	ocPosID    = ecs.RegisterComp[ocPos]()
	ocHealthID = ecs.RegisterComp[ocHealth]()
	ocWeaponID = ecs.RegisterComp[ocWeapon]()
	ocCrownID  = ecs.RegisterComp[ocCrown]()
	ocLootID   = ecs.RegisterComp[ocLoot]()
)

// registerClassSchemas registers a small object family in group "oc":
// Hero{Pos, Health, ?Weapon}, Boss{Pos, Health, Crown} (a superset of Hero's
// required set), and Chest/Crate with identical component sets (ambiguous).
func registerClassSchemas(t *testing.T) {
	t.Helper()
	weapon := testComponent[ocWeapon]("Weapon", ocWeaponID)
	weapon.Optional = true
	RegisterSchema(Schema{SchemaMeta: SchemaMeta{Group: "oc", Class: "Hero"}, Components: []Component{
		testComponent[ocPos]("Pos", ocPosID), testComponent[ocHealth]("Health", ocHealthID), weapon,
	}})
	RegisterSchema(Schema{SchemaMeta: SchemaMeta{Group: "oc", Class: "Boss"}, Components: []Component{
		testComponent[ocPos]("Pos", ocPosID), testComponent[ocHealth]("Health", ocHealthID), testComponent[ocCrown]("Crown", ocCrownID),
	}})
	for _, class := range []string{"Chest", "Crate"} {
		RegisterSchema(Schema{SchemaMeta: SchemaMeta{Group: "oc", Class: class}, Components: []Component{
			testComponent[ocLoot]("Loot", ocLootID),
		}})
	}
}

func TestSchemas(t *testing.T) {
	registerClassSchemas(t)
	var keys []string
	for s := range Schemas() {
		keys = append(keys, s.Key())
	}
	if !slices.IsSorted(keys) {
		t.Fatalf("Schemas not sorted: %v", keys)
	}
	for _, want := range []string{"oc:Boss", "oc:Chest", "oc:Crate", "oc:Hero"} {
		if !slices.Contains(keys, want) {
			t.Fatalf("Schemas lacks %s: %v", want, keys)
		}
	}

	// Snapshot: registering during iteration neither deadlocks nor shows up.
	n := 0
	for range Schemas() {
		if n == 0 {
			RegisterSchema(Schema{SchemaMeta: SchemaMeta{Group: "oc", Class: "Late"}, Components: []Component{
				testComponent[ocLoot]("Loot", ocLootID),
			}})
		}
		n++
	}
	if n != len(keys) {
		t.Fatalf("iteration saw %d schemas, want the %d of the snapshot", n, len(keys))
	}

	// Early break.
	for range Schemas() {
		break
	}
}

func TestClassTag(t *testing.T) {
	registerClassSchemas(t)
	tag, ok := ClassTag("oc", "Hero")
	if !ok {
		t.Fatal("ClassTag(oc, Hero) not found")
	}
	if got := tag.Meta(); got != (SchemaMeta{Group: "oc", Class: "Hero"}) {
		t.Fatalf("Meta = %+v", got)
	}
	if _, ok := ClassTag("oc", "Nope"); ok {
		t.Fatal("unknown class found")
	}
	if (ObjectClass{}).Meta() != (SchemaMeta{}) {
		t.Fatal("zero ObjectClass must have zero Meta")
	}

	// Identical re-registration keeps the index.
	registerClassSchemas(t)
	if again, _ := ClassTag("oc", "Hero"); again != tag {
		t.Fatalf("re-registration changed the tag: %v != %v", again, tag)
	}
}

func TestClassOf(t *testing.T) {
	registerClassSchemas(t)
	w := ecs.NewWorld()
	heroTag, _ := ClassTag("oc", "Hero")

	for _, tc := range []struct {
		name   string
		values []any
		want   string // "" means not resolved
	}{
		{"tagged", []any{ocLoot{}, heroTag}, "oc:Hero"},
		{"untagged exact", []any{ocPos{}, ocHealth{}}, "oc:Hero"},
		{"untagged with optional", []any{ocPos{}, ocHealth{}, ocWeapon{}}, "oc:Hero"},
		{"superset is the richer object", []any{ocPos{}, ocHealth{}, ocCrown{}}, "oc:Boss"},
		{"tie", []any{ocLoot{}}, ""},
		{"no candidate", []any{ocPos{}}, ""},
		{"no components", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := w.NewEntity(tc.values...)
			meta, ok := ClassOf(w, e)
			if tc.want == "" {
				if ok {
					t.Fatalf("ClassOf = %s, want unresolved", meta.Key())
				}
				return
			}
			if !ok || meta.Key() != tc.want {
				t.Fatalf("ClassOf = %s, %v; want %s", meta.Key(), ok, tc.want)
			}
		})
	}

	stale := w.NewEntity(ocPos{})
	w.DeleteEntity(stale)
	mustPanic(t, "ecs: invalid or stale entity", func() { ClassOf(w, stale) })
}

func TestObjects(t *testing.T) {
	registerClassSchemas(t)
	w := ecs.NewWorld()
	doc := `{"objects":[` +
		`{"group":"oc","class":"Hero","components":{"t:Pos":{"X":1,"Y":2},"t:Health":{"HP":3}}},` +
		`{"group":"oc","class":"Boss","components":{"t:Pos":{"X":4,"Y":5},"t:Health":{"HP":6},"t:Crown":{"Gold":7}}}]}`
	objs, err := Deserialize(w, strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range objs {
		if !w.HasComp(o.Entity, objectClassID) {
			t.Fatalf("%s from Deserialize has no ObjectClass", o.Key())
		}
	}
	w.NewEntity(ocPos{}) // not an object
	untagged := w.NewEntity(ocPos{X: 8}, ocHealth{HP: 9})

	got := slices.Collect(Objects(w))
	if len(got) != 3 {
		t.Fatalf("Objects = %+v, want 3 objects", got)
	}
	if !slices.ContainsFunc(got, func(o Object) bool { return o.Entity == untagged && o.Class == "Hero" }) {
		t.Fatalf("Objects lacks the untagged hero: %+v", got)
	}

	var buf strings.Builder
	if err := Serialize(w, &buf, got...); err != nil {
		t.Fatal(err)
	}
	w2 := ecs.NewWorld()
	again, err := Deserialize(w2, strings.NewReader(buf.String()))
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 3 {
		t.Fatalf("round trip gave %d objects", len(again))
	}
	var buf2 strings.Builder
	if err := Serialize(w2, &buf2, again...); err != nil {
		t.Fatal(err)
	}
	if buf.String() != buf2.String() {
		t.Fatalf("round trip differs:\n%s\n%s", buf.String(), buf2.String())
	}

	for range Objects(w) {
		break // early break must not panic
	}
}

package gen

import (
	"strings"
	"testing"
)

// TestDiagnostics checks that generate() reports every validation rule
// violated in testdata/diag, each with the exact file:line:col: message
// text, sorted and deduplicated into one error.
func TestDiagnostics(t *testing.T) {
	_, err := generate("testdata/diag", []string{"."})
	if err == nil {
		t.Fatal("expected diagnostics, got nil error")
	}
	got := strings.Split(err.Error(), "\n")

	want := []string{
		"default.go:29:2: diag.DefObject.C: +game:default is only valid on a field of a component struct (or a struct nested in one), not on an object field",
		"default.go:31:2: diag.DefObject.Plain: +game:default is only valid on a field of a component struct (or a struct nested in one), not on an object field",
		"default.go:37:2: diag.BadDefaults.WrongType: +game:default: \"x\" cannot be used as float64",
		"default.go:39:2: diag.BadDefaults.Overflow: +game:default: 300 overflows uint8",
		"default.go:41:2: diag.BadDefaults.Fraction: +game:default: 1.5 cannot be used as int",
		"default.go:43:2: diag.BadDefaults.Unknown: +game:default: undefined: Missing",
		"default.go:45:2: diag.BadDefaults.NotConstant: +game:default: f() is not a constant",
		"default.go:47:2: diag.BadDefaults.NotAMember: +game:default: undefined: LevelMissing",
		"default.go:49:2: diag.BadDefaults.BadMember: +game:default: 7 is not a value of the enum (0 LevelLow, 1 LevelHigh)",
		"default.go:51:2: diag.BadDefaults.WrongConstType: +game:default: OtherA has type diag.Other, want diag.Level",
		"default.go:54:2: diag.BadDefaults.Entity: +game:default is not allowed on an entity reference (ids belong to one document)",
		"default.go:56:2: diag.BadDefaults.Runtime: +game:default is not allowed on a runtime field (not authored data)",
		"default.go:59:2: diag.BadDefaults.Asset: +game:default is not allowed on an asset field (a new component starts without an asset)",
		"default.go:62:2: diag.BadDefaults.Hidden: +game:default is not allowed on a field with json:\"-\" (it is not in the schema)",
		"default.go:64:2: diag.BadDefaults.unexported: +game:default is not allowed on an unexported field (it is not in the schema)",
		"default.go:66:2: diag.BadDefaults.Counter: +game:default is not allowed on a value whose JSON shape is not described",
		"default.go:69:2: diag.BadDefaults.Struct: +game:default: a struct needs its fields set by path (+game:default:name=value)",
		"default.go:71:2: diag.BadDefaults.NoField: +game:default:Z: DefVec has no field Z",
		"default.go:73:2: diag.BadDefaults.OnScalar: +game:default:X.Y: cannot select Y in a scalar",
		"default.go:76:2: diag.BadDefaults.Twice: +game:default sets path X more than once",
		"default.go:78:2: marker +game:default: duplicate path \"X\"",
		"default.go:80:2: diag.BadDefaults.Mixed: +game:default mixes the field form (=value) with the path form (:path=value)",
		"default.go:83:2: diag.BadDefaults.ScalarPath: +game:default:X: cannot select X in a scalar",
		"default.go:85:2: diag.BadDefaults.SlicePath: +game:default:0: cannot select 0 in a slice or array",
		"default.go:87:2: diag.BadDefaults.ArrayLen: +game:default: the array has 3 elements, the list 2",
		"default.go:89:2: diag.BadDefaults.ListNeeded: +game:default: expected a list {a, b, ...}, got 1",
		"default.go:91:2: diag.BadDefaults.StructList: +game:default: a list of diag.DefVec cannot be written (elements must be scalars)",
		"default.go:93:2: diag.BadDefaults.MapOfSlices: +game:default:a: expected a list {a, b, ...}, got 1",
		"default.go:95:2: diag.BadDefaults.MapDeep: +game:default:a.b: a map of structs or maps cannot be set by path (key a)",
		"default.go:97:2: diag.BadDefaults.IntKeys: +game:default:x: key \"x\" is not a valid int",
		"default.go:99:2: marker +game:default: needs a value (=value or :path=value)",
		"default.go:101:2: marker +game:default: missing value",
		"diag.go:16:6: diag.BothMarkers: a type cannot be both +game:object and +game:component",
		"diag.go:24:6: diag.NotAStruct: +game:object and +game:component must be applied to a struct type",
		"diag.go:29:6: diag.Generic: generic types are not supported",
		"diag.go:39:2: diag.NestedMarkerComponent.Inner: component type must not contain +game:component or +game:embed fields",
		"diag.go:47:2: diag.UnresolvedComponent.Bad: component type does not resolve",
		"diag.go:55:2: diag.NestedPointer.V: component type must be T or *T, not a nested pointer",
		"diag.go:63:2: diag.UnnamedComponent.V: component type must be a named type, got []int",
		"diag.go:71:2: diag.InterfaceComponent.V: component type must not be an interface",
		"diag.go:91:2: diag.UsesForeignType.V: component type foreign.hiddenType is unexported and cannot be named from package diag",
		"diag.go:100:2: diag.ComponentAndEmbed.V: +game:component and +game:embed are mutually exclusive",
		"diag.go:108:2: diag.BareOptional.V: +game:optional requires +game:component on the same field",
		"diag.go:122:2: diag.OptionalEmbed.Embeddable: +game:optional requires +game:component on the same field",
		"diag.go:131:2: diag.AnonymousComponent.V1: +game:component requires a named field (embedded fields use +game:embed)",
		"diag.go:140:2: diag.OptionalValue.V: +game:optional is not allowed on a value component (absent and zero are indistinguishable)",
		"diag.go:149:2: diag.EmbedNonStruct.Name: +game:embed field must be a +game:object struct (or a pointer to one)",
		"diag.go:162:2: diag.EmbedGenericInstance.Box: embedding a generic object is not supported",
		"diag.go:174:2: diag.EmbedNotStruct.NotStructNamed: embedded type NotStructNamed must be a struct declared with +game:object",
		"diag.go:187:2: diag.EmbedUnmarked.PlainStruct: embedded type PlainStruct is not marked +game:object",
		"diag.go:196:3: diag.CycleA.CycleB: embed cycle CycleB -> CycleA -> CycleB",
		"diag.go:202:3: diag.CycleB.CycleA: embed cycle CycleA -> CycleB -> CycleA",
		"diag.go:213:2: diag.DuplicateComponentType: component type V1 is used twice: A and B",
		"diag.go:227:2: diag.NameCollision: field name Foo collides after flattening: HasFooA.Foo and HasFooB.Foo",
		"diag.go:250:6: diag.DupKeyB: object key diag:DupKey is already used by DupKeyA",
		"diag.go:263:2: marker +game:asset: missing required argument \"kind\"",
		"diag.go:266:2: diag.BadAssetFields.NoTag: +game:asset requires the struct tag json:\"-\" (the field is runtime-only; the schema carries its property)",
		"diag.go:268:2: diag.BadAssetFields.Pointer: +game:asset requires a struct handle type (not a pointer or non-struct), got *Handle",
		"diag.go:270:2: diag.BadAssetFields.NotStruct: +game:asset requires a struct handle type (not a pointer or non-struct), got int",
		"diag.go:272:2: diag.BadAssetFields.unexported: +game:asset requires an exported field",
		"diag.go:275:2: diag.BadAssetFields.Both: +game:asset and +game:runtime are mutually exclusive",
		"diag.go:277:2: diag.BadAssetFields.Handle: +game:asset is not allowed on an embedded field",
		"diag.go:279:2: diag.BadAssetFields.RuntimeNoTag: +game:runtime requires the struct tag json:\"-\" (the field is runtime-only; the schema carries its property)",
		"diag.go:288:3: diag.NestedAsset.H: +game:asset is only valid on a direct field of a component struct, not on objects or nested, inlined or non-component structs",
		"diag.go:295:2: diag.ObjectWithAsset.H: +game:asset is only valid on a direct field of a component struct, not on objects or nested, inlined or non-component structs",
		"diag.go:301:2: diag.NotAComponent.T: +game:runtime is only valid on a direct field of a component struct, not on objects or nested, inlined or non-component structs",
		"editor.go:12:2: marker +game:editor: argument \"min\": invalid float64 \"x\"",
		"editor.go:14:2: diag.BadEditor.Inverted: +game:editor min 5 is above max 1",
		"editor.go:16:2: diag.BadEditor.ZeroStep: +game:editor step must be above 0",
		"editor.go:18:2: marker +game:editor: argument \"order\": invalid int \"first\"",
		"editor.go:20:2: diag.BadEditor.OnString: +game:editor min, max, step and unit=deg need a number, or an array or object of numbers",
		"editor.go:22:2: diag.BadEditor.OnBool: +game:editor min, max, step and unit=deg need a number, or an array or object of numbers",
		"editor.go:24:2: diag.BadEditor.NotColor: +game:editor color needs an integer (0xRRGGBBAA), an object with R, G, B (and A) or an array of 3 or 4 numbers",
		"editor.go:26:2: diag.BadEditor.WrongArray: +game:editor color needs an integer (0xRRGGBBAA), an object with R, G, B (and A) or an array of 3 or 4 numbers",
		"editor.go:28:2: diag.BadEditor.NoSibling: +game:editor showIf \"Missing\": \"Missing\" is not another property of the same struct (use its JSON name)",
		"editor.go:30:2: diag.BadEditor.BadEnum: +game:editor showIf \"Mode=LevelNope\": \"LevelNope\" is not a constant of the sibling's enum",
		"editor.go:32:2: diag.BadEditor.BadBool: +game:editor showIf \"Flag=maybe\": a boolean is compared with true or false",
		"editor.go:34:2: diag.BadEditor.BadInt: +game:editor showIf \"Count=many\": \"many\" is not an integer",
		"editor.go:36:2: diag.BadEditor.OnSelf: +game:editor showIf \"OnSelf\": \"OnSelf\" is not another property of the same struct (use its JSON name)",
		"editor.go:38:2: marker +game:editor: unknown argument \"bogus\"",
		"editor.go:40:2: diag.BadEditor.Skipped: +game:editor is not allowed on a field with json:\"-\" (it is not in the schema)",
		"editor.go:42:2: diag.BadEditor.unexported: +game:editor is not allowed on an unexported field (it is not in the schema)",
		"enum.go:5:1: diag.EnumString: +game:enum requires a named integer type, got string",
		"enum.go:10:1: diag.EnumEmpty: +game:enum has no exported constants of type EnumEmpty",
		"enum.go:17:1: diag.EnumNoZero: +game:enum has no constant with value 0; the zero value would not validate",
		"enum.go:29:2: diag.EnumAlias: +game:enum constants EnumAliasA and EnumAliasB have the same value 0",
		"enum.go:34:1: diag.EnumJSON: +game:enum on a type with MarshalJSON or MarshalText; its wire value is not the integer",
		"enum.go:45:2: marker +game:enum is not valid on a field",
		"foreign/foreign.go:9:2: foreign.Base.hp: field hp is unexported and cannot be accessed from package diag",
		"identity.go:9:2: diag.UsesUngrouped.V: component Plain (github.com/weakpixel/engine/tools/internal/gen/testdata/diag/nogroup) is used by object diag:UsesUngrouped but has no group; add +game:component:group=... or +game:group:name to its package",
		"identity.go:17:6: diag.ClashB: component key diag:Clash is already used by github.com/weakpixel/engine/tools/internal/gen/testdata/diag.ClashA",
		"identity.go:20:1: diag.BadIdentity: +game:component group \"a-b\" is not a valid identifier",
		"identity.go:20:1: diag.BadIdentity: +game:component name \"1x\" is not a valid identifier",
		"identity.go:26:2: diag.FieldArgs.V: +game:component takes group and name on the component type, not on a field",
	}

	if len(got) != len(want) {
		t.Fatalf("got %d diagnostics, want %d:\n--- got ---\n%s\n--- want ---\n%s",
			len(got), len(want), strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("diagnostic %d:\n got:  %s\n want: %s", i, got[i], want[i])
		}
	}
}

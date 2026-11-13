// Package gametypes provides Game Object schemas shared by tests.
package gametypes

import (
	"iter"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/god"
)

// Transform stores an entity's world position.
type Transform struct{ X, Y float64 }

// Acceleration stores an entity's acceleration.
type Acceleration struct{ X, Y float64 }

// State stores an entity's activity and behavior kind.
type State struct {
	Active bool
	Kind   string
}

// Health stores an entity's current and maximum hit points.
type Health struct{ Current, Max int64 }

// Vel stores an entity's velocity.
type Vel struct{ X, Y float64 }

// Weapon stores damage and ammunition.
type Weapon struct {
	Damage float64
	Ammo   int64
}

// Attachment is a nested value in Companion.
type Attachment struct{ Name string }

// Companion is a readonly value component.
type Companion struct {
	Name       string
	Attachment *Attachment
}

// Armor stores damage reduction.
type Armor struct{ Value int64 }

// Follow stores an entity reference and following distance.
type Follow struct {
	Target   ecs.Entity
	Distance float64
}

// Hitbox stores a local rectangle.
type Hitbox struct {
	Width, Height, OffsetX, OffsetY float64
}

// Common contains components shared by Hero and Boss.
type Common struct {
	Transform    *Transform
	Acceleration *Acceleration
	State        *State
}

// Hero is a player-like object schema.
type Hero struct {
	Common
	Health    *Health
	Vel       *Vel
	Hitbox    *Hitbox
	Companion Companion
	Weapon    *Weapon
	Follow    *Follow
}

// Boss is a Hero-like object with required armor.
type Boss struct {
	Common    *Common
	Health    *Health
	Vel       *Vel
	Hitbox    *Hitbox
	Companion Companion
	Weapon    *Weapon
	Armor     *Armor
}

var (
	transformID    = ecs.RegisterComp[Transform]()
	accelerationID = ecs.RegisterComp[Acceleration]()
	stateID        = ecs.RegisterComp[State]()
	healthID       = ecs.RegisterComp[Health]()
	velID          = ecs.RegisterComp[Vel]()
	weaponID       = ecs.RegisterComp[Weapon]()
	companionID    = ecs.RegisterComp[Companion]()
	armorID        = ecs.RegisterComp[Armor]()
	followID       = ecs.RegisterComp[Follow]()
	hitboxID       = ecs.RegisterComp[Hitbox]()
)

func init() {
	transform := registerComponent[Transform]("Transform", transformID, objectSchema(
		[]string{"X", "Y"},
		map[string]*god.TypeSchema{"X": numberSchema(), "Y": numberSchema()},
	))
	acceleration := registerComponent[Acceleration]("Acceleration", accelerationID, objectSchema(
		[]string{"X", "Y"},
		map[string]*god.TypeSchema{"X": numberSchema(), "Y": numberSchema()},
	))
	state := registerComponent[State]("State", stateID, objectSchema(
		[]string{"Active", "Kind"},
		map[string]*god.TypeSchema{"Active": {Type: "boolean"}, "Kind": {Type: "string"}},
	))
	health := registerComponent[Health]("Health", healthID, objectSchema(
		[]string{"Current", "Max"},
		map[string]*god.TypeSchema{"Current": integerSchema(), "Max": integerSchema()},
	))
	vel := registerComponent[Vel]("Vel", velID, objectSchema(
		[]string{"X", "Y"},
		map[string]*god.TypeSchema{"X": numberSchema(), "Y": numberSchema()},
	))
	weapon := registerComponent[Weapon]("Weapon", weaponID, objectSchema(
		[]string{"Damage", "Ammo"},
		map[string]*god.TypeSchema{"Damage": numberSchema(), "Ammo": integerSchema()},
	))
	weapon.Optional = true
	companion := registerComponent[Companion]("Companion", companionID, objectSchema(
		[]string{"Name"},
		map[string]*god.TypeSchema{
			"Name": {Type: "string"},
			"Attachment": {
				Type: "object", Nullable: true,
				Properties: map[string]*god.TypeSchema{"Name": {Type: "string"}},
				Required:   []string{"Name"},
			},
		},
	))
	companion.Readonly = true
	armor := registerComponent[Armor]("Armor", armorID, objectSchema(
		[]string{"Value"},
		map[string]*god.TypeSchema{"Value": integerSchema()},
	))
	follow := registerComponent[Follow]("Follow", followID, objectSchema(
		[]string{"Distance"},
		map[string]*god.TypeSchema{
			"Target":   {Type: "string", Format: "entity", Nullable: true},
			"Distance": numberSchema(),
		},
	))
	follow.Optional = true
	hitbox := registerComponent[Hitbox]("Hitbox", hitboxID, objectSchema(
		[]string{"Width", "Height", "OffsetX", "OffsetY"},
		map[string]*god.TypeSchema{
			"Width": numberSchema(), "Height": numberSchema(),
			"OffsetX": numberSchema(), "OffsetY": numberSchema(),
		},
	))

	god.RegisterSchema(god.Schema{
		SchemaMeta: god.SchemaMeta{Group: "game", Class: "Hero"},
		Components: []god.Component{
			transform, acceleration, state, health, vel, hitbox, companion, weapon, follow,
		},
	})
	god.RegisterSchema(god.Schema{
		SchemaMeta: god.SchemaMeta{Group: "game", Class: "Boss"},
		Components: []god.Component{
			transform, acceleration, state, health, vel, hitbox, companion, weapon, armor,
		},
	})
}

// registerComponent adds a component to the runtime and returns its object-schema entry.
func registerComponent[T any](name string, id ecs.ComponentID, schema *god.TypeSchema) god.Component {
	god.RegisterComponent(god.ComponentSchema{
		Group: "game", Name: name, RuntimeID: id, Schema: schema, Decode: god.DecodeFunc[T](),
	})
	return god.Component{
		Group: "game", Name: name, Field: name, RuntimeID: id, Schema: schema,
		Decode: god.DecodeFunc[T](),
	}
}

func objectSchema(required []string, properties map[string]*god.TypeSchema) *god.TypeSchema {
	return &god.TypeSchema{Type: "object", Required: required, Properties: properties}
}

func numberSchema() *god.TypeSchema {
	return &god.TypeSchema{Type: "number", Format: "double"}
}

func integerSchema() *god.TypeSchema {
	return &god.TypeSchema{Type: "integer", Format: "int64"}
}

type HeroView struct{ world *ecs.World }

// NewHeroView binds a Hero view to world.
func NewHeroView(world *ecs.World) HeroView { return HeroView{world: world} }

// NewEntity creates a Hero and returns its entity.
func (v HeroView) NewEntity(obj Hero) ecs.Entity {
	values := []any{
		*obj.Transform, *obj.Acceleration, *obj.State, *obj.Health, *obj.Vel, *obj.Hitbox, obj.Companion,
	}
	if obj.Weapon != nil {
		values = append(values, *obj.Weapon)
	}
	if obj.Follow != nil {
		values = append(values, *obj.Follow)
	}
	return v.world.NewEntity(values...)
}

// Get reads the components of e as a Hero.
func (v HeroView) Get(e ecs.Entity) Hero {
	return Hero{
		Common: Common{
			Transform:    ecs.GetComp[Transform](v.world, e),
			Acceleration: ecs.GetComp[Acceleration](v.world, e),
			State:        ecs.GetComp[State](v.world, e),
		},
		Health:    ecs.GetComp[Health](v.world, e),
		Vel:       ecs.GetComp[Vel](v.world, e),
		Hitbox:    ecs.GetComp[Hitbox](v.world, e),
		Companion: *ecs.GetComp[Companion](v.world, e),
		Weapon:    ecs.GetCompOrNil[Weapon](v.world, e),
		Follow:    ecs.GetCompOrNil[Follow](v.world, e),
	}
}

// Each yields every entity with the required Hero components.
func (v HeroView) Each() iter.Seq2[ecs.Entity, Hero] {
	return func(yield func(ecs.Entity, Hero) bool) {
		for e := range v.world.Query(accelerationID, companionID, healthID, hitboxID, stateID, transformID, velID).Each() {
			if !yield(e, v.Get(e)) {
				return
			}
		}
	}
}

type BossView struct{ world *ecs.World }

// NewBossView binds a Boss view to world.
func NewBossView(world *ecs.World) BossView { return BossView{world: world} }

// NewEntity creates a Boss and returns its entity.
func (v BossView) NewEntity(obj Boss) ecs.Entity {
	return v.world.NewEntity(
		*obj.Common.Transform, *obj.Common.Acceleration, *obj.Common.State,
		*obj.Health, *obj.Vel, *obj.Hitbox, obj.Companion, *obj.Armor,
	)
}

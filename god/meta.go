package god

// SchemaMeta identifies a game object for external systems.
type SchemaMeta struct {
	// Group is the group to which the object belongs.
	Group string `json:"group"`

	// Class is the class within the group.
	Class string `json:"class"`
}

// Key returns the registry key "group:class". Groups may contain ':' (import
// paths do not, but explicit values are free-form), so splitting uses the last ':'.
func (m SchemaMeta) Key() string {
	return m.Group + ":" + m.Class
}

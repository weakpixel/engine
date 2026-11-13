package marker

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// Parse parses text (`+name` or `+name:arg=val,...`, without the leading
// comment slashes) for a declaration of the given single target. It returns
// the marker's full name and a new value of its definition struct type.
//
// With arguments the text is read to its end; without, parsing stops at the
// first whitespace and trailing text is ignored.
func (r *Registry) Parse(target TargetType, text string) (string, any, error) {
	if !target.single() {
		return "", nil, fmt.Errorf("invalid target %d", int(target))
	}
	body, ok := strings.CutPrefix(text, "+")
	if !ok {
		return "", nil, errNotMarker
	}
	def, rest := r.match(body)
	if def == nil {
		name, _, _ := strings.Cut(body, " ")
		name, _, _ = strings.Cut(name, "\t")
		return "", nil, fmt.Errorf("unknown marker +%s", name)
	}
	if def.Target&target == 0 {
		return "", nil, fmt.Errorf("marker +%s is not valid on a %s", def.Name, target)
	}
	val := reflect.New(def.typ).Elem()
	var err error
	switch {
	case def.paths:
		var args PathArgs
		if args, err = parsePathArgs(rest); err == nil {
			val.Set(reflect.ValueOf(args))
		}
	case rest != "" && rest[0] == '=':
		err = fmt.Errorf("unexpected =value (use +%s:arg=value)", def.Name)
	case rest != "" && rest[0] == ':':
		err = def.parseArgs(val, rest[1:])
	default:
		err = def.parseArgs(val, "")
	}
	if err == nil && def.validate != nil {
		err = def.validate(val.Interface())
	}
	if err != nil {
		return "", nil, fmt.Errorf("marker +%s: %w", def.Name, err)
	}
	return def.Name, val.Interface(), nil
}

// parseArgs fills val from a comma-separated `name=value` list and checks
// that all required arguments are present. An empty list is only valid when
// args is "" and nothing is required.
func (d *Definition) parseArgs(val reflect.Value, args string) error {
	set := map[string]bool{}
	for s := args; s != ""; {
		i := strings.IndexAny(s, "=,")
		if i < 0 || s[i] == ',' {
			name, _, _ := strings.Cut(s, ",")
			return fmt.Errorf("argument %q needs a value (name=value)", strings.TrimSpace(name))
		}
		name := strings.TrimSpace(s[:i])
		a := d.arg(name)
		switch {
		case name == "":
			return fmt.Errorf("missing argument name")
		case a == nil:
			return fmt.Errorf("unknown argument %q", name)
		case set[name]:
			return fmt.Errorf("duplicate argument %q", name)
		}
		set[name] = true
		v, rest, err := parseValue(a.typ, s[i+1:])
		if err != nil {
			return fmt.Errorf("argument %q: %w", name, err)
		}
		val.Field(a.index).Set(v)
		if rest = strings.TrimSpace(rest); rest != "" {
			if rest[0] != ',' {
				return fmt.Errorf("argument %q: unexpected %q", name, rest)
			}
			if rest = strings.TrimSpace(rest[1:]); rest == "" {
				return fmt.Errorf("missing argument after ','")
			}
		}
		s = rest
	}
	for _, a := range d.args {
		if !a.optional && !set[a.name] {
			return fmt.Errorf("missing required argument %q", a.name)
		}
	}
	return nil
}

// parseValue reads one argument value of type t from s and returns the
// unconsumed rest.
func parseValue(t reflect.Type, s string) (reflect.Value, string, error) {
	s = strings.TrimLeft(s, " \t")
	if t.Kind() == reflect.Pointer {
		elem, rest, err := parseValue(t.Elem(), s)
		if err != nil {
			return reflect.Value{}, "", err
		}
		p := reflect.New(t.Elem())
		p.Elem().Set(elem)
		return p, rest, nil
	}
	if t.Kind() != reflect.Slice {
		return parseScalar(t, s, ",;")
	}
	out := reflect.MakeSlice(t, 0, 0)
	if rest, ok := strings.CutPrefix(s, "{"); ok {
		s = rest
		for {
			s = strings.TrimLeft(s, " \t")
			if s == "" {
				return out, "", fmt.Errorf("unterminated '{'")
			}
			if s[0] == '}' {
				return out, s[1:], nil
			}
			elem, rest, err := parseScalar(t.Elem(), s, ",}")
			if err != nil {
				return out, "", err
			}
			out = reflect.Append(out, elem)
			s = strings.TrimLeft(rest, " \t")
			switch {
			case s == "":
				return out, "", fmt.Errorf("unterminated '{'")
			case s[0] == ',':
				s = s[1:]
			case s[0] != '}':
				return out, "", fmt.Errorf("expected ',' or '}' in list")
			}
		}
	}
	for {
		elem, rest, err := parseScalar(t.Elem(), s, ",;")
		if err != nil {
			return out, "", err
		}
		out = reflect.Append(out, elem)
		rest = strings.TrimLeft(rest, " \t")
		if rest == "" || rest[0] != ';' {
			return out, rest, nil
		}
		s = strings.TrimLeft(rest[1:], " \t")
	}
}

// parseScalar reads one value of type t. Strings may be Go quoted or raw;
// everything else (and bare strings) runs up to one of the stop characters.
func parseScalar(t reflect.Type, s, stops string) (reflect.Value, string, error) {
	v := reflect.New(t).Elem()
	if t.Kind() == reflect.String && s != "" && (s[0] == '"' || s[0] == '`') {
		q, err := strconv.QuotedPrefix(s)
		if err != nil {
			return v, "", fmt.Errorf("unterminated or invalid quoted string")
		}
		str, err := strconv.Unquote(q)
		if err != nil {
			return v, "", fmt.Errorf("invalid quoted string %s", q)
		}
		v.SetString(str)
		return v, s[len(q):], nil
	}
	tok, rest := s, ""
	if i := strings.IndexAny(s, stops); i >= 0 {
		tok, rest = s[:i], s[i:]
	}
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return v, "", fmt.Errorf("missing value")
	}
	switch t.Kind() {
	case reflect.String:
		v.SetString(tok)
	case reflect.Bool:
		b, err := strconv.ParseBool(tok)
		if err != nil {
			return v, "", fmt.Errorf("invalid bool %q", tok)
		}
		v.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(tok, 0, t.Bits())
		if err != nil {
			return v, "", fmt.Errorf("invalid %s %q", t.Kind(), tok)
		}
		v.SetInt(n)
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(tok, t.Bits())
		if err != nil {
			return v, "", fmt.Errorf("invalid %s %q", t.Kind(), tok)
		}
		v.SetFloat(n)
	default:
		n, err := strconv.ParseUint(tok, 0, t.Bits())
		if err != nil {
			return v, "", fmt.Errorf("invalid %s %q", t.Kind(), tok)
		}
		v.SetUint(n)
	}
	return v, rest, nil
}

// PathArgs is the parsed form of a path marker (Registry.DefinePaths). Values
// are raw text: a Go expression the caller evaluates.
type PathArgs struct {
	Bare    string      // the value of `+name=value`; valid when HasBare
	HasBare bool        // the marker had the `=value` form
	Paths   []PathValue // the `path=value` pairs of `+name:path=value,...`, in order
}

// PathValue is one `path=value` pair. Path is dotted names; each segment is a
// Go field name or a map key.
type PathValue struct {
	Path  string
	Value string
}

// parsePathArgs reads what follows the marker name: `=value` or
// `:path=value[,path=value...]`.
func parsePathArgs(rest string) (PathArgs, error) {
	var out PathArgs
	if v, ok := strings.CutPrefix(rest, "="); ok {
		out.HasBare, out.Bare = true, strings.TrimSpace(v)
		if out.Bare == "" {
			return out, fmt.Errorf("missing value")
		}
		return out, nil
	}
	s, ok := strings.CutPrefix(rest, ":")
	if !ok || strings.TrimSpace(s) == "" {
		return out, fmt.Errorf("needs a value (=value or :path=value)")
	}
	seen := map[string]bool{}
	for s != "" {
		path, after, ok := strings.Cut(s, "=")
		path = strings.TrimSpace(path)
		switch {
		case !ok:
			return out, fmt.Errorf("path %q needs a value (path=value)", path)
		case !validPath(path):
			return out, fmt.Errorf("invalid path %q", path)
		case seen[path]:
			return out, fmt.Errorf("duplicate path %q", path)
		}
		seen[path] = true
		val, rest := splitValue(after)
		if val = strings.TrimSpace(val); val == "" {
			return out, fmt.Errorf("path %q: missing value", path)
		}
		out.Paths = append(out.Paths, PathValue{path, val})
		if rest == "" {
			break
		}
		if s = strings.TrimSpace(rest[1:]); s == "" {
			return out, fmt.Errorf("missing path after ','")
		}
	}
	return out, nil
}

// validPath reports whether p is dotted segments of letters, digits, '_' and '-'.
func validPath(p string) bool {
	for seg := range strings.SplitSeq(p, ".") {
		if !validName(seg, false) {
			return false
		}
	}
	return true
}

// splitValue cuts s at the first ',' outside quotes and brackets and returns
// the value and the rest (starting at the comma, or empty).
func splitValue(s string) (val, rest string) {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '"', '`', '\'':
			if q, err := strconv.QuotedPrefix(s[i:]); err == nil {
				i += len(q) - 1
			} else if j := strings.IndexByte(s[i+1:], c); j >= 0 {
				i += j + 1 // a quote the Go parser rejects: let the evaluator report it
			}
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth <= 0 {
				return s[:i], s[i:]
			}
		}
	}
	return s, ""
}

// SplitList splits a `{a, b, c}` list value into its element texts. Elements
// are cut at commas outside quotes and brackets; a trailing comma and `{}`
// are allowed.
func SplitList(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	inner, ok := strings.CutPrefix(s, "{")
	if ok {
		inner, ok = strings.CutSuffix(inner, "}")
	}
	if !ok {
		return nil, fmt.Errorf("expected a list {a, b, ...}, got %s", s)
	}
	var out []string
	for rest := strings.TrimSpace(inner); rest != ""; {
		val, after := splitValue(rest)
		if val = strings.TrimSpace(val); val == "" {
			return nil, fmt.Errorf("empty list element")
		}
		out = append(out, val)
		rest = strings.TrimSpace(strings.TrimPrefix(after, ","))
	}
	return out, nil
}

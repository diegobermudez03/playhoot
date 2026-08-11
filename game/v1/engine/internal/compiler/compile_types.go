package compiler

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

// compileTypeDeclarations compiles every registered type name, in
// source order, into its resolved engine.Type.
//
// It skips a source index that registerTypeNamespace already diagnosed
// (an empty/unsupported name, or a duplicate) — only the canonical,
// first-registered index for each name is compiled — so iterating in
// source order here, rather than over the typeDeclarations map directly,
// is what keeps compilation, and therefore diagnostic order, independent
// of Go's unspecified map iteration order.
func (c *compiler) compileTypeDeclarations() map[string]engine.Type {
	result := make(map[string]engine.Type, len(c.typeDeclarations))
	for i, t := range c.definition.Types {
		name, ok := typeDeclarationName(t)
		if !ok || name == "" {
			continue
		}
		entry, registered := c.typeDeclarations[name]
		if !registered || entry.index != i {
			continue
		}
		result[name] = c.resolveNamedType(name)
	}
	return result
}

// resolveNamedType returns the compiled engine.Type for the registered
// type name, compiling and memoizing it on first use. It assumes name is
// already registered in c.typeDeclarations; callers resolving a
// reference to a type by name must check existence, and check
// c.resolvingTypes for a cycle, before calling this.
func (c *compiler) resolveNamedType(name string) engine.Type {
	if t, ok := c.resolvedTypes[name]; ok {
		return t
	}
	entry := c.typeDeclarations[name]

	c.resolvingTypes[name] = true
	t := c.compileTypeDeclaration(entry.decl, entry.path)
	delete(c.resolvingTypes, name)

	c.resolvedTypes[name] = t
	return t
}

func (c *compiler) compileTypeDeclaration(decl program.TypeDeclaration, path string) engine.Type {
	switch d := decl.(type) {
	case program.EnumTypeDeclaration:
		return c.compileEnumType(d, path)
	case program.RecordTypeDeclaration:
		return c.compileRecordType(d, path)
	case program.UnionTypeDeclaration:
		return c.compileUnionType(d, path)
	case program.NewTypeDeclaration:
		return c.compileNewType(d, path)
	default:
		c.addf(path, "missing or unsupported type declaration")
		return nil
	}
}

func (c *compiler) compileEnumType(d program.EnumTypeDeclaration, path string) engine.Type {
	valuesPath := path + ".values"
	values := make([]string, 0, len(d.Values))
	seen := make(map[string]int, len(d.Values))
	for i, v := range d.Values {
		valuePath := fmt.Sprintf("%s[%d]", valuesPath, i)
		if v.Name == "" {
			c.addf(valuePath, "enum value has an empty name")
			continue
		}
		if first, ok := seen[v.Name]; ok {
			c.addf(valuePath, "duplicate enum value name %q (first declared at %s[%d])", v.Name, valuesPath, first)
			continue
		}
		seen[v.Name] = i
		values = append(values, v.Name)
	}
	return engine.EnumType{Name: d.Name, Values: values}
}

func (c *compiler) compileRecordType(d program.RecordTypeDeclaration, path string) engine.Type {
	return engine.RecordType{
		Name:   d.Name,
		Fields: c.compileFieldDeclarations(d.Fields, path+".fields"),
	}
}

func (c *compiler) compileUnionType(d program.UnionTypeDeclaration, path string) engine.Type {
	variantsPath := path + ".variants"
	variants := make([]engine.UnionVariantType, 0, len(d.Variants))
	seen := make(map[string]int, len(d.Variants))
	for i, v := range d.Variants {
		variantPath := fmt.Sprintf("%s[%d]", variantsPath, i)
		if v.Name == "" {
			c.addf(variantPath, "union variant has an empty name")
			continue
		}
		if first, ok := seen[v.Name]; ok {
			c.addf(variantPath, "duplicate union variant name %q (first declared at %s[%d])", v.Name, variantsPath, first)
			continue
		}
		seen[v.Name] = i
		variants = append(variants, engine.UnionVariantType{
			Name:   v.Name,
			Fields: c.compileFieldDeclarations(v.Fields, variantPath+".fields"),
		})
	}
	return engine.UnionType{Name: d.Name, Variants: variants}
}

func (c *compiler) compileNewType(d program.NewTypeDeclaration, path string) engine.Type {
	return engine.NewType{
		Name:       d.Name,
		Underlying: c.compileTypeReference(d.Underlying, path+".underlying"),
	}
}

// compileFieldDeclarations compiles fields, diagnosing an empty or
// duplicate field name and skipping it — keeping only the first
// occurrence — the same way registerTypeNamespace does for type names.
// It is shared by RecordType and every UnionVariantType, since both
// declare a plain, independently namespaced list of fields.
func (c *compiler) compileFieldDeclarations(fields []program.FieldDeclaration, pathPrefix string) []engine.FieldType {
	result := make([]engine.FieldType, 0, len(fields))
	seen := make(map[string]int, len(fields))
	for i, f := range fields {
		fieldPath := fmt.Sprintf("%s[%d]", pathPrefix, i)
		if f.Name == "" {
			c.addf(fieldPath, "field has an empty name")
			continue
		}
		if first, ok := seen[f.Name]; ok {
			c.addf(fieldPath, "duplicate field name %q (first declared at %s[%d])", f.Name, pathPrefix, first)
			continue
		}
		seen[f.Name] = i
		result = append(result, engine.FieldType{
			Name: f.Name,
			Type: c.compileTypeReference(f.Type, fieldPath+".type"),
		})
	}
	return result
}

// compileTypeReference resolves ref into its compiled engine.Type. A
// named reference to a type this compiler is already in the middle of
// compiling — directly, or through another type, a list, a map, or an
// optional — is diagnosed as recursive here, at the reference site,
// rather than by resolveNamedType, so the diagnostic points at the
// specific reference that closes the cycle.
func (c *compiler) compileTypeReference(ref program.TypeReference, path string) engine.Type {
	switch t := ref.(type) {
	case nil:
		c.addf(path, "missing type reference")
		return nil
	case program.BuiltinTypeReference:
		return c.compileBuiltinType(t.Type, path)
	case program.NamedTypeReference:
		if t.Name == "" {
			c.addf(path, "named type reference has an empty name")
			return nil
		}
		if _, ok := c.typeDeclarations[t.Name]; !ok {
			c.addf(path, "reference to undeclared type %q", t.Name)
			return nil
		}
		if c.resolvingTypes[t.Name] {
			c.addf(path, "type %q is defined recursively (directly or through a list, map, or optional), which this compiler does not yet support", t.Name)
			return nil
		}
		return c.resolveNamedType(t.Name)
	case program.ListTypeReference:
		return engine.ListType{Element: c.compileTypeReference(t.Element, path+".element")}
	case program.MapTypeReference:
		return engine.MapType{
			Key:   c.compileTypeReference(t.Key, path+".key"),
			Value: c.compileTypeReference(t.Value, path+".value"),
		}
	case program.OptionalTypeReference:
		return engine.OptionalType{Element: c.compileTypeReference(t.Element, path+".element")}
	default:
		c.addf(path, "unsupported type reference")
		return nil
	}
}

func (c *compiler) compileBuiltinType(b program.BuiltinType, path string) engine.Type {
	switch b {
	case program.BuiltinTypeUnit:
		return engine.UnitType{}
	case program.BuiltinTypeBool:
		return engine.BoolType{}
	case program.BuiltinTypeNumber:
		return engine.NumberType{}
	case program.BuiltinTypeString:
		return engine.StringType{}
	case program.BuiltinTypeUser:
		return engine.UserType{}
	default:
		c.addf(path, "unknown built-in type %q", b)
		return nil
	}
}

package engine

// Kind identifies the shape category shared by a Value and a Type: every
// Value has exactly one Kind, and every Type describes values of exactly
// one Kind. For a Value that Validate accepts against some Type, both
// report the same Kind.
type Kind int

const (
	KindUnit Kind = iota
	KindBool
	KindNumber
	KindString
	KindUser
	KindEnum
	KindRecord
	KindUnion
	KindNewType
	KindOptional
	KindList
	KindMap
)

func (k Kind) String() string {
	switch k {
	case KindUnit:
		return "unit"
	case KindBool:
		return "bool"
	case KindNumber:
		return "number"
	case KindString:
		return "string"
	case KindUser:
		return "user"
	case KindEnum:
		return "enum"
	case KindRecord:
		return "record"
	case KindUnion:
		return "union"
	case KindNewType:
		return "new_type"
	case KindOptional:
		return "optional"
	case KindList:
		return "list"
	case KindMap:
		return "map"
	default:
		return "unknown"
	}
}

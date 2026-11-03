package gameversion

// Language is a language a game version is written in, one of Playhoot's
// languages (BCP 47 tags). The frontend's SDK keeps the same list
// (game-core's gameLanguages); language_test.go checks both stay equal.
type Language string

// Languages are the languages Playhoot is offered in, in the platform's order.
var Languages = []Language{"es", "en", "pt-BR"}

// ValidLanguage reports whether l is one of Playhoot's languages.
func ValidLanguage(l Language) bool {
	for _, known := range Languages {
		if l == known {
			return true
		}
	}
	return false
}

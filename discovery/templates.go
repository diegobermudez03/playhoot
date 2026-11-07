// Package discovery is how people and AIs find games: the catalog of public
// games, replicated from Game Management, and the template catalog an AI
// starts a game from.
package discovery

// TemplateEntry is one template as the catalog lists it: data the database
// keeps about the game, never written by an AI. Players, languages, aspect
// ratio and input requirements are its published version's.
type TemplateEntry struct {
	GameUUID         string
	Name             string
	Description      string
	Categories       []string
	PlayersMin       int
	PlayersMax       int
	Languages        []string
	AspectRatio      string
	UsesKeyboard     bool
	UsesPointerHover bool
	Featured         bool
}

// TemplateCategory is a category templates are filed under, with how many.
type TemplateCategory struct {
	Name      string
	Templates int
}

// TemplateQuery is what a template search asks for: words to match, a
// category, a number of players the template must accept, and a page (from
// 1). Empty fields do not filter.
type TemplateQuery struct {
	Text     string
	Category string
	Players  int
	Page     int
}

// TemplatePage is one page of a search, and whether more follow.
type TemplatePage struct {
	Templates []TemplateEntry
	More      bool
}

// MaxTemplatesPage is how many templates one page lists.
const MaxTemplatesPage = 20

package loginproviders

import (
	"bytes"
	"context"
	"html/template"
	"net/url"
	"strings"

	"github.com/diegobermudez03/playhoot/identity/internal/business/displayname"
)

// LocalIssuer is the issuer of every identity the local provider vouches for.
const LocalIssuer = "https://local.playhoot.invalid"

// LocalConfig places the local provider's two endpoints.
type LocalConfig struct {
	// AuthorizeURL serves AuthorizePage; CallbackURL is the login callback the
	// page answers.
	AuthorizeURL string
	CallbackURL  string
}

// Local is a fictitious provider for development and tests: it vouches for
// whatever name the person picks. The same name is always the same subject,
// so signing in again resolves the same account.
type Local struct {
	config LocalConfig
}

// localPeople are offered on the page, so most sign-ins are one click.
var localPeople = []string{"Ana", "Beto", "Carla"}

// NewLocal constructs the local provider.
func NewLocal(config LocalConfig) *Local {
	return &Local{config: config}
}

// AuthorizationURL implements Provider.
func (l *Local) AuthorizationURL(state string, _ string) string {
	return l.config.AuthorizeURL + "?" + url.Values{"state": {state}}.Encode()
}

// Exchange implements Provider: the code is the name the person picked.
func (l *Local) Exchange(_ context.Context, code string, _ string) (ExternalIdentity, error) {
	name, ok := displayname.Normalize(code)
	if !ok {
		return ExternalIdentity{}, ErrInvalidAnswer
	}
	slug := strings.ToLower(name)
	return ExternalIdentity{
		Issuer:  LocalIssuer,
		Subject: "local:" + slug,
		Email:   strings.ReplaceAll(slug, " ", ".") + "@local.playhoot.invalid",
		Name:    name,
	}, nil
}

var localPage = template.Must(template.New("local").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="referrer" content="no-referrer">
<title>Local sign-in (development)</title>
<style>
body { font: 16px system-ui, sans-serif; margin: 0; padding: 24px 16px; background: #f4f1fb; color: #1d1530; }
main { max-width: 420px; margin: 0 auto; }
h1 { font-size: 20px; }
p { color: #5b5270; }
a, button { display: block; width: 100%; box-sizing: border-box; margin: 8px 0; padding: 12px; border-radius: 10px;
  border: 1px solid #c9bfe3; background: #fff; color: inherit; font: inherit; text-align: center; text-decoration: none; cursor: pointer; }
input { width: 100%; box-sizing: border-box; padding: 12px; border-radius: 10px; border: 1px solid #c9bfe3; font: inherit; }
.cancel { background: transparent; border-style: dashed; }
</style>
</head>
<body>
<main>
<h1>Local sign-in</h1>
<p>A fictitious provider for development. The same name is always the same account.</p>
{{range .People}}<a href="{{.Href}}">{{.Name}}</a>
{{end}}<form method="get" action="{{.Callback}}">
<input type="hidden" name="state" value="{{.State}}">
<input name="code" maxlength="64" required placeholder="Another name" aria-label="Another name">
<button type="submit">Sign in with this name</button>
</form>
<a class="cancel" href="{{.Cancel}}">Cancel</a>
</main>
</body>
</html>
`))

type localPerson struct {
	Name string
	// Href is built from encoded values only, so it is a trusted URL.
	Href template.URL
}

// AuthorizePage is the page AuthorizeURL serves for the attempt with state.
func (l *Local) AuthorizePage(state string) string {
	people := make([]localPerson, 0, len(localPeople))
	for _, name := range localPeople {
		people = append(people, localPerson{Name: name, Href: l.callback(url.Values{"code": {name}, "state": {state}})})
	}
	var page bytes.Buffer
	// The template and its data are fixed: executing it cannot fail.
	_ = localPage.Execute(&page, map[string]any{
		"People":   people,
		"Callback": l.config.CallbackURL,
		"State":    state,
		"Cancel":   l.callback(url.Values{"error": {"access_denied"}, "state": {state}}),
	})
	return page.String()
}

// callback is a link to the callback: a configured path and encoded values
// only, so the template may trust it.
func (l *Local) callback(query url.Values) template.URL {
	return template.URL(l.config.CallbackURL + "?" + query.Encode())
}

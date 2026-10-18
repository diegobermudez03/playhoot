package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// internalCitationPattern matches citations of internal decision documents
// (ADR, WORK, Blocker, Slice, docs paths) used in place of a reason.
var internalCitationPattern = regexp.MustCompile(`(?i)\b(?:[A-Z]+-)?ADR-\d+\b|\bWORK-\d{4}\b|\bBlocker \d+\b|\bSlice \d+\b|\bdocs/(?:work|engineering|ai)/`)

// TestNoInternalDocCitationsInComments walks every tracked .go file's
// comments and fails if any cites an internal ADR/WORK/Blocker/Slice
// reference or docs-tree path, catching the mistake mechanically instead
// of relying solely on code review to notice it.
func TestNoInternalDocCitationsInComments(t *testing.T) {
	var violations []string

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Vendored third-party code follows its own conventions.
			if info.Name() == ".git" || info.Name() == "third_party" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
		for _, group := range file.Comments {
			for _, comment := range group.List {
				if internalCitationPattern.MatchString(comment.Text) {
					violations = append(violations, fmt.Sprintf("%s: %s", fset.Position(comment.Pos()), strings.TrimSpace(comment.Text)))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking repository for .go files: %v", err)
	}

	if len(violations) > 0 {
		t.Errorf("found %d comment(s) citing an internal ADR/WORK/Blocker/Slice/docs-path reference as justification - state the reasoning directly instead (see the engineering standards directory's code-comments doc):\n%s",
			len(violations), strings.Join(violations, "\n"))
	}
}

// lowerLayerPackageIdentifiers are engine/infrastructure packages a public doc
// comment must not send the reader to; extend it, with a reason, when a new
// such layer appears.
var lowerLayerPackageIdentifiers = map[string]string{
	"engine":        "Game Language's execution engine - session's own public contract exists specifically so callers never need it",
	"engineservice": "the engine's own service/codec layer - same reason as engine",
	"gorm":          "the ORM library used for persistence - an implementation detail of whichever repository happens to use it",
}

// qualifiedSymbolPattern matches a package-qualified identifier reference
// inside a comment, e.g. "engineservice.EncodeValue" or
// "internalrepo.TimerObligation".
var qualifiedSymbolPattern = regexp.MustCompile(`\b([a-z][a-zA-Z0-9]*)\.[A-Z][a-zA-Z0-9]*\b`)

// qualifiedIdentsInType collects the package identifiers used in a
// declaration's signature; a comment naming those adds no hidden dependency.
func qualifiedIdentsInType(expr ast.Node) map[string]bool {
	idents := map[string]bool{}
	if expr == nil {
		return idents
	}
	ast.Inspect(expr, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if pkgIdent, ok := sel.X.(*ast.Ident); ok {
				idents[pkgIdent.Name] = true
			}
		}
		return true
	})
	return idents
}

// receiverIsExportedOrAbsent reports whether recv (a FuncDecl's receiver
// field list) is absent (a plain function, not a method) or names an
// exported receiver type - a pointer or value receiver whose base type
// identifier starts with an uppercase letter.
func receiverIsExportedOrAbsent(recv *ast.FieldList) bool {
	if recv == nil || len(recv.List) == 0 {
		return true
	}
	recvType := recv.List[0].Type
	if star, ok := recvType.(*ast.StarExpr); ok {
		recvType = star.X
	}
	ident, ok := recvType.(*ast.Ident)
	return ok && ast.IsExported(ident.Name)
}

// TestExportedDocCommentsStayAtPublicContract fails when an exported doc
// comment names an internal/ or lower-layer package its signature does not
// already expose. Narration that names no package is left to code review.
func TestExportedDocCommentsStayAtPublicContract(t *testing.T) {
	var violations []string

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "internal" || info.Name() == "third_party" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			return fmt.Errorf("parsing %s: %w", path, parseErr)
		}

		importPathByIdent := make(map[string]string, len(file.Imports))
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			ident := importPath[strings.LastIndex(importPath, "/")+1:]
			if imp.Name != nil {
				ident = imp.Name.Name
			}
			importPathByIdent[ident] = importPath
		}

		check := func(doc *ast.CommentGroup, name string, signatureType ast.Node) {
			if doc == nil || !ast.IsExported(name) {
				return
			}
			alreadyInSignature := qualifiedIdentsInType(signatureType)
			for _, match := range qualifiedSymbolPattern.FindAllStringSubmatch(doc.Text(), -1) {
				ident := match[1]
				if alreadyInSignature[ident] {
					continue
				}
				importPath, imported := importPathByIdent[ident]
				if !imported {
					continue
				}
				reason, denied := lowerLayerPackageIdentifiers[ident]
				isInternal := strings.Contains(importPath, "/internal/")
				if !denied && !isInternal {
					continue
				}
				if denied {
					violations = append(violations, fmt.Sprintf("%s: %s's doc comment references %s.* (%s)", fset.Position(doc.Pos()), name, ident, reason))
				} else {
					violations = append(violations, fmt.Sprintf("%s: %s's doc comment references %s.* (%s is an internal/ package, unimportable outside its module tree)", fset.Position(doc.Pos()), name, ident, importPath))
				}
			}
		}

		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if !receiverIsExportedOrAbsent(d.Recv) {
					// An exported method on an unexported receiver type is
					// not reachable from outside the package (the type
					// itself can never be named/constructed there), so it
					// is not part of any real public contract.
					continue
				}
				check(d.Doc, d.Name.Name, d.Type)
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						doc := s.Doc
						if doc == nil {
							doc = d.Doc
						}
						check(doc, s.Name.Name, s.Type)
					case *ast.ValueSpec:
						doc := s.Doc
						if doc == nil {
							doc = d.Doc
						}
						for _, n := range s.Names {
							check(doc, n.Name, s.Type)
						}
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking repository for .go files: %v", err)
	}

	if len(violations) > 0 {
		t.Errorf("found %d exported doc comment(s) referencing a lower-layer/internal package not otherwise part of the declaration's own signature - describe the public contract self-sufficiently instead (see the engineering standards directory's code-comments doc, \"Public API Comments Stay At The Public Contract\"):\n%s",
			len(violations), strings.Join(violations, "\n"))
	}
}

// maxDocCommentLines bounds every doc comment; detail that does not fit
// belongs in docs, tests or clearer code.
const maxDocCommentLines = 5

// docCommentLines counts a doc comment's lines, leaving out tool directives
// such as //go:generate.
func docCommentLines(fset *token.FileSet, doc *ast.CommentGroup) int {
	lines := 0
	for _, comment := range doc.List {
		if strings.HasPrefix(comment.Text, "//go:") || strings.HasPrefix(comment.Text, "//nolint") {
			continue
		}
		lines += fset.Position(comment.End()).Line - fset.Position(comment.Pos()).Line + 1
	}
	return lines
}

// TestDocCommentLength fails on any doc comment (package, declaration or
// struct field) longer than maxDocCommentLines, in every non-generated file.
func TestDocCommentLength(t *testing.T) {
	var violations []string

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Vendored third-party code follows its own conventions.
			if info.Name() == ".git" || info.Name() == "third_party" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			return fmt.Errorf("parsing %s: %w", path, parseErr)
		}
		if ast.IsGenerated(file) {
			return nil
		}

		check := func(doc *ast.CommentGroup, name string) {
			if doc == nil {
				return
			}
			if lines := docCommentLines(fset, doc); lines > maxDocCommentLines {
				violations = append(violations, fmt.Sprintf("%s: %s (%d lines)", fset.Position(doc.Pos()), name, lines))
			}
		}

		check(file.Doc, "package "+file.Name.Name)
		ast.Inspect(file, func(n ast.Node) bool {
			switch d := n.(type) {
			case *ast.FuncDecl:
				check(d.Doc, d.Name.Name)
			case *ast.GenDecl:
				check(d.Doc, d.Tok.String())
			case *ast.TypeSpec:
				check(d.Doc, d.Name.Name)
			case *ast.ValueSpec:
				check(d.Doc, d.Names[0].Name)
			case *ast.Field:
				name := "field"
				if len(d.Names) > 0 {
					name = d.Names[0].Name
				}
				check(d.Doc, name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking repository for .go files: %v", err)
	}

	if len(violations) > 0 {
		t.Errorf("found %d doc comment(s) longer than %d lines - keep the contract to 1-3 lines and move detail to docs or tests (see the engineering standards directory's code-comments doc):\n%s",
			len(violations), maxDocCommentLines, strings.Join(violations, "\n"))
	}
}

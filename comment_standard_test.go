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

// internalCitationPattern matches the shapes of internal decision-tracking
// reference this repository's own engineering comment standard forbids
// citing as the reason something is true in a source comment: an ADR
// number, a WORK number, a Blocker/Slice reference, or a path into this
// repository's own docs tree. The actual reasoning must be written out in
// plain language instead of pointing at a document for it.
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
			if info.Name() == ".git" {
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

// lowerLayerPackageIdentifiers names package identifiers this repository's
// own domains use for engine/runtime/infrastructure-layer implementation -
// never part of any domain's own public contract. An exported doc comment
// referencing one of these (e.g. "see engineservice.EncodeValue") asks the
// reader to understand a layer the public symbol exists to abstract away,
// even though the package itself is perfectly importable (not Go-internal).
// Extend this list, with a one-line reason, whenever another such layer
// appears - for example, a future domain's own internal execution engine.
// This list only matters for a reference NOT already part of the
// declaration's own signature - see qualifiedIdentsInType below.
var lowerLayerPackageIdentifiers = map[string]string{
	"engine":        "Game Language's execution engine - game/session's own public contract exists specifically so callers never need it",
	"engineservice": "the engine's own service/codec layer - same reason as engine",
	"gorm":          "the ORM library used for persistence - an implementation detail of whichever repository happens to use it",
}

// qualifiedSymbolPattern matches a package-qualified identifier reference
// inside a comment, e.g. "engineservice.EncodeValue" or
// "internalrepo.TimerObligation".
var qualifiedSymbolPattern = regexp.MustCompile(`\b([a-z][a-zA-Z0-9]*)\.[A-Z][a-zA-Z0-9]*\b`)

// qualifiedIdentsInType collects every package identifier referenced by a
// SelectorExpr (pkg.Symbol) anywhere inside expr - the type expression of a
// declaration's own signature (its parameters/results, or a type alias's
// underlying type). A package already appearing here is already part of
// what the declaration's exported signature forces any caller to see - a
// doc comment naming it too is not introducing a new hidden dependency, it
// is describing an already-unavoidable one.
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

// TestExportedDocCommentsStayAtPublicContract walks every tracked,
// non-test .go file outside any internal/ directory and fails if an
// exported top-level declaration's doc comment references a
// package-qualified symbol whose import path contains "/internal/", or
// whose local package identifier is on lowerLayerPackageIdentifiers -
// unless that same package already appears in the declaration's own
// signature (see qualifiedIdentsInType), in which case the caller cannot
// avoid seeing it regardless of the comment.
//
// This mechanically catches the concrete half of this repository's "Public
// API Comments Stay At The Public Contract" standard: a public method's own
// doc comment must never send the reader to a lower architectural layer
// merely to understand how to call it, when that layer is not otherwise
// forced on the caller by the signature itself. It cannot catch the softer
// half (implementation narration that names no package) - that remains a
// code-review responsibility.
func TestExportedDocCommentsStayAtPublicContract(t *testing.T) {
	var violations []string

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "internal" {
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

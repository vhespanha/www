package ssg

import (
	"html"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// excerptLen is the most characters an excerpt keeps: about what search
// engines show of a description.
const excerptLen = 160

// excerpt returns the plain text of the first paragraph of doc, cut at a word
// boundary to at most excerptLen characters.
func excerpt(doc ast.Node, src []byte) string {
	var first ast.Node
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if _, ok := n.(*ast.Paragraph); ok && entering {
			first = n
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	if first == nil {
		return ""
	}
	var b strings.Builder
	_ = ast.Walk(first, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Text:
			b.Write(n.Segment.Value(src))
			if n.SoftLineBreak() || n.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			// The typographer's replacements are HTML entities.
			b.WriteString(html.UnescapeString(string(n.Value)))
		case *ast.RawHTML, *extast.FootnoteLink:
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	s := strings.Join(strings.Fields(b.String()), " ")
	if utf8.RuneCountInString(s) <= excerptLen {
		return s
	}
	cut := string([]rune(s)[:excerptLen])
	if i := strings.LastIndexByte(cut, ' '); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:.") + "…"
}

// headerScopes marks the header cells of tables as column headers, so that
// screen readers announce them with the cells below.
type headerScopes struct{}

func (headerScopes) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if c, ok := n.(*extast.TableCell); ok && entering {
			if _, ok := c.Parent().(*extast.TableHeader); ok {
				c.SetAttributeString("scope", "col")
			}
		}
		return ast.WalkContinue, nil
	})
}

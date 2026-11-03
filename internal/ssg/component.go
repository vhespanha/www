package ssg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/a-h/templ"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"go.yaml.in/yaml/v3"
)

// A Component is markup that notes embed with a fenced code block whose info
// string is "@" followed by the component's name. The block's body is YAML
// setting the component's props:
//
//	```@counter
//	start: 3
//	```
//
// An unknown name or prop is an error.
type Component struct {
	render func(props []byte) (templ.Component, error)
	head   templ.Component
}

// NewComponent returns a component that decodes its props into a P and
// renders them with render. If head is not nil, it is added once to the head
// of each page using the component, for example to load a script.
func NewComponent[P any](render func(P) templ.Component, head templ.Component) Component {
	return Component{
		render: func(props []byte) (templ.Component, error) {
			var p P
			dec := yaml.NewDecoder(bytes.NewReader(props))
			dec.KnownFields(true)
			if err := dec.Decode(&p); err != nil && !errors.Is(err, io.EOF) {
				return nil, fmt.Errorf("props: %w", err)
			}
			return render(p), nil
		},
		head: head,
	}
}

// componentUses records, while converting one note, the components it uses.
type componentUses struct {
	seen  map[string]bool
	heads []templ.Component
	err   error
}

var usesKey = parser.NewContextKey()

var kindComponent = ast.NewNodeKind("Component")

// componentNode is a block replaced by a component's rendered HTML.
type componentNode struct {
	ast.BaseBlock
	html []byte
}

func (n *componentNode) Kind() ast.NodeKind { return kindComponent }

func (n *componentNode) Dump(src []byte, level int) { ast.DumpHelper(n, src, level, nil, nil) }

// componentTransformer replaces fenced code blocks calling a component with
// the component's HTML.
type componentTransformer map[string]Component

func (t componentTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	uses := pc.Get(usesKey).(*componentUses)
	src := reader.Source()
	var blocks []*ast.FencedCodeBlock
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if b, ok := n.(*ast.FencedCodeBlock); ok && entering && strings.HasPrefix(string(b.Language(src)), "@") {
			blocks = append(blocks, b)
		}
		return ast.WalkContinue, nil
	})
	for _, b := range blocks {
		name := strings.TrimPrefix(string(b.Language(src)), "@")
		html, err := t.render(uses, name, blockBody(b, src))
		if err != nil {
			uses.err = fmt.Errorf("component %q: %w", name, err)
			return
		}
		b.Parent().ReplaceChild(b.Parent(), b, &componentNode{html: html})
	}
}

func (t componentTransformer) render(uses *componentUses, name string, props []byte) ([]byte, error) {
	c, ok := t[name]
	if !ok {
		return nil, errors.New("unknown component")
	}
	comp, err := c.render(props)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := comp.Render(context.Background(), &buf); err != nil {
		return nil, err
	}
	if !uses.seen[name] {
		if uses.seen == nil {
			uses.seen = make(map[string]bool)
		}
		uses.seen[name] = true
		if c.head != nil {
			uses.heads = append(uses.heads, c.head)
		}
	}
	return buf.Bytes(), nil
}

func blockBody(b *ast.FencedCodeBlock, src []byte) []byte {
	var body []byte
	lines := b.Lines()
	for i := range lines.Len() {
		seg := lines.At(i)
		body = append(body, seg.Value(src)...)
	}
	return body
}

type componentRenderer struct{}

func (componentRenderer) RegisterFuncs(r renderer.NodeRendererFuncRegisterer) {
	r.Register(kindComponent, func(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if _, err := w.Write(n.(*componentNode).html); err != nil {
				return ast.WalkStop, err
			}
		}
		return ast.WalkSkipChildren, nil
	})
}

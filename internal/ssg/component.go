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
func NewComponent[P any](
	render func(P) templ.Component,
	head templ.Component,
) Component {
	return Component{
		render: func(props []byte) (templ.Component, error) {
			var p P
			if err := decodeProps(props, &p); err != nil {
				return nil, err
			}
			return render(p), nil
		},
		head: head,
	}
}

// decodeProps unmarshals the YAML props into v, rejecting unknown fields. No
// props leave v unchanged.
func decodeProps(props []byte, v any) error {
	dec := yaml.NewDecoder(bytes.NewReader(props))
	dec.KnownFields(true)
	err := dec.Decode(v)
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("props: %w", err)
	}
	return nil
}

// componentUses records, while converting one note, the components it uses.
type componentUses struct {
	seen  map[string]bool
	heads []templ.Component
	err   error
}

var usesKey = parser.NewContextKey()

// add records that the note uses c, under name.
func (u *componentUses) add(name string, c Component) {
	if u.seen[name] {
		return
	}
	if u.seen == nil {
		u.seen = make(map[string]bool)
	}
	u.seen[name] = true
	if c.head != nil {
		u.heads = append(u.heads, c.head)
	}
}

var kindComponent = ast.NewNodeKind("Component")

// componentNode is a block replaced by a component's rendered HTML.
type componentNode struct {
	ast.BaseBlock
	html []byte
}

func (n *componentNode) Kind() ast.NodeKind { return kindComponent }

func (n *componentNode) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, nil, nil)
}

// componentTransformer replaces fenced code blocks calling a component with
// the component's HTML.
type componentTransformer map[string]Component

func (t componentTransformer) Transform(
	doc *ast.Document,
	reader text.Reader,
	pc parser.Context,
) {
	uses := pc.Get(usesKey).(*componentUses)
	src := reader.Source()
	for _, b := range componentCalls(doc, src) {
		name, _ := calledName(b, src)
		html, err := t.render(name, blockBody(b, src))
		if err != nil {
			uses.err = fmt.Errorf("component %q: %w", name, err)
			return
		}
		uses.add(name, t[name])
		parent := b.Parent()
		parent.ReplaceChild(parent, b, &componentNode{html: html})
	}
}

// render renders the component named name with the YAML props.
func (t componentTransformer) render(name string, props []byte) ([]byte, error) {
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
	return buf.Bytes(), nil
}

// componentCalls returns the fenced code blocks in doc that call a component.
// They are collected before any is replaced, so the walk never sees a changed
// tree.
func componentCalls(doc ast.Node, src []byte) []*ast.FencedCodeBlock {
	var calls []*ast.FencedCodeBlock
	walk := func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		b, ok := n.(*ast.FencedCodeBlock)
		if !ok || !entering {
			return ast.WalkContinue, nil
		}
		if _, ok := calledName(b, src); ok {
			calls = append(calls, b)
		}
		return ast.WalkContinue, nil
	}
	_ = ast.Walk(doc, walk)
	return calls
}

// calledName returns the name of the component that b calls, which its info
// string gives after an "@", and whether it calls one at all.
func calledName(b *ast.FencedCodeBlock, src []byte) (string, bool) {
	return strings.CutPrefix(string(b.Language(src)), "@")
}

// blockBody returns the contents of b.
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
	r.Register(kindComponent, renderComponent)
}

// renderComponent writes a componentNode's HTML.
func renderComponent(
	w util.BufWriter,
	_ []byte,
	n ast.Node,
	entering bool,
) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	if _, err := w.Write(n.(*componentNode).html); err != nil {
		return ast.WalkStop, err
	}
	return ast.WalkSkipChildren, nil
}

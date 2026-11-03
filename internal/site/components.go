package site

import "go.vhespanha.dev/www/internal/ssg"

// components are the components notes can embed, by name. See [ssg.Component].
var components = map[string]ssg.Component{
	"counter": ssg.NewComponent(counter, script("/assets/components/counter.js")),
}

type counterProps struct {
	Start int `yaml:"start"`
}

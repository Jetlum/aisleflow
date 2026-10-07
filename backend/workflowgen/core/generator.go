package core

import (
	"bytes"
	_ "embed"
	"fmt"
	"go/format"
	"go/token"
	"strconv"
	"text/template"
)

//go:embed templates/workflow.go.tmpl
var workflowTemplate string

//go:embed templates/schema.go.tmpl
var schemaTemplate string

type templateData struct {
	Process
	Package, Start string
	Tasks          []Node
	Out            map[string][]Flow
}

// Generate returns both complete, gofmt-validated artifacts before any file is written.
func Generate(p Process, pkg string) (workflow, schema []byte, err error) {
	if err = Validate(p); err != nil {
		return
	}
	if !token.IsIdentifier(pkg) || token.Lookup(pkg).IsKeyword() || pkg == "_" {
		return nil, nil, fmt.Errorf("invalid Go package %q", pkg)
	}
	d := templateData{Process: p, Package: pkg, Out: map[string][]Flow{}}
	for _, n := range p.Nodes {
		if n.Kind == "startEvent" {
			d.Start = n.ID
		}
		if n.Kind == "serviceTask" {
			d.Tasks = append(d.Tasks, n)
		}
	}
	if len(d.Tasks) == 0 {
		return nil, nil, fmt.Errorf("at least one service task required for schema generation")
	}
	for _, f := range p.Flows {
		d.Out[f.Source] = append(d.Out[f.Source], f)
	}
	render := func(name, source string) ([]byte, error) {
		t, err := template.New(name).Funcs(template.FuncMap{"q": strconv.Quote}).Parse(source)
		if err != nil {
			return nil, err
		}
		var b bytes.Buffer
		if err = t.Execute(&b, d); err != nil {
			return nil, err
		}
		return format.Source(b.Bytes())
	}
	workflow, err = render("workflow", workflowTemplate)
	if err != nil {
		return
	}
	schema, err = render("schema", schemaTemplate)
	return
}

// Package core implements a deliberately small, fail-closed BPMN compiler.
package core

import (
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"

	"example.com/aisleflow/backend/common/contracts"
)

const ModelNS = "http://www.omg.org/spec/BPMN/20100524/MODEL"
const ExtensionNS = "https://aisleflow.dev/bpmn/v1"
const MaxXMLBytes = 1 << 20

type Node struct {
	ID, Kind, Aisle, Alternative, Variable string
}
type Flow struct{ ID, Source, Target, Value string }
type Process struct {
	ID    string
	Nodes []Node
	Flows []Flow
}

// Parse uses namespace-aware token parsing. Unknown executable semantics are errors,
// never silently discarded. Diagram interchange and documentation are non-executable.
func Parse(r io.Reader) (Process, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxXMLBytes+1))
	if err != nil {
		return Process{}, err
	}
	if len(data) > MaxXMLBytes {
		return Process{}, fmt.Errorf("BPMN exceeds 1 MiB")
	}
	d := xml.NewDecoder(strings.NewReader(string(data)))
	var p Process
	var stack []xml.Name
	processes, roots := 0, 0
	for {
		t, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return p, fmt.Errorf("XML: %w", err)
		}
		switch t := t.(type) {
		case xml.Directive:
			return p, fmt.Errorf("XML directives/DOCTYPE are unsupported")
		case xml.CharData:
			if strings.TrimSpace(string(t)) != "" {
				return p, fmt.Errorf("unexpected text in executable BPMN")
			}
		case xml.StartElement:
			depth := len(stack)
			if depth == 0 {
				roots++
				if roots != 1 || t.Name != (xml.Name{Space: ModelNS, Local: "definitions"}) {
					return p, fmt.Errorf("expected one BPMN definitions root")
				}
			} else if t.Name.Space == ModelNS && t.Name.Local == "documentation" {
				if err := d.Skip(); err != nil {
					return p, err
				}
				continue
			} else if depth == 1 && t.Name.Space == "http://www.omg.org/spec/BPMN/20100524/DI" && t.Name.Local == "BPMNDiagram" {
				if err := d.Skip(); err != nil {
					return p, err
				}
				continue
			} else if depth == 1 && t.Name == (xml.Name{Space: ModelNS, Local: "process"}) {
				processes++
				p.ID = attr(t, "", "id")
				if err := attributes(t, "id", "name", "isExecutable"); err != nil {
					return p, err
				}
				if attr(t, "", "isExecutable") != "true" {
					return p, fmt.Errorf("process must explicitly declare isExecutable=true")
				}
			} else if depth == 2 && t.Name.Space == ModelNS {
				switch t.Name.Local {
				case "startEvent", "endEvent", "serviceTask", "exclusiveGateway":
					if err := attributes(t, "id", "name"); err != nil {
						return p, err
					}
					p.Nodes = append(p.Nodes, Node{ID: attr(t, "", "id"), Kind: t.Name.Local, Aisle: attr(t, ExtensionNS, "aisle"), Alternative: attr(t, ExtensionNS, "alternative"), Variable: attr(t, ExtensionNS, "variable")})
				case "sequenceFlow":
					if err := attributes(t, "id", "name", "sourceRef", "targetRef"); err != nil {
						return p, err
					}
					p.Flows = append(p.Flows, Flow{ID: attr(t, "", "id"), Source: attr(t, "", "sourceRef"), Target: attr(t, "", "targetRef"), Value: attr(t, ExtensionNS, "value")})
				default:
					return p, fmt.Errorf("unsupported BPMN element %s", t.Name.Local)
				}
			} else {
				return p, fmt.Errorf("unsupported nested/foreign element %s", t.Name.Local)
			}
			stack = append(stack, t.Name)
		case xml.EndElement:
			if len(stack) == 0 {
				return p, fmt.Errorf("unexpected end element")
			}
			stack = stack[:len(stack)-1]
		}
	}
	if roots != 1 || processes != 1 {
		return p, fmt.Errorf("exactly one process is required")
	}
	if err := Validate(p); err != nil {
		return p, err
	}
	sort.Slice(p.Nodes, func(i, j int) bool { return p.Nodes[i].ID < p.Nodes[j].ID })
	sort.Slice(p.Flows, func(i, j int) bool { return p.Flows[i].ID < p.Flows[j].ID })
	return p, nil
}

func attr(e xml.StartElement, ns, key string) string {
	for _, a := range e.Attr {
		if a.Name.Space == ns && a.Name.Local == key {
			return a.Value
		}
	}
	return ""
}
func attributes(e xml.StartElement, allowed ...string) error {
	seen := map[xml.Name]bool{}
	for _, a := range e.Attr {
		if seen[a.Name] {
			return fmt.Errorf("duplicate attribute %s", a.Name.Local)
		}
		seen[a.Name] = true
		if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" {
			continue
		}
		ok := false
		if a.Name.Space == "" {
			for _, s := range allowed {
				if a.Name.Local == s {
					ok = true
				}
			}
		}
		if a.Name.Space == ExtensionNS {
			switch e.Name.Local {
			case "serviceTask":
				ok = a.Name.Local == "aisle" || a.Name.Local == "alternative"
			case "exclusiveGateway":
				ok = a.Name.Local == "variable"
			case "sequenceFlow":
				ok = a.Name.Local == "value"
			}
		}
		if !ok {
			return fmt.Errorf("unsupported attribute %s on %s", a.Name.Local, e.Name.Local)
		}
	}
	return nil
}

func Validate(p Process) error {
	if !contracts.ValidID(p.ID) {
		return fmt.Errorf("invalid process ID")
	}
	if len(p.Nodes) > 500 {
		return fmt.Errorf("maximum 500 nodes")
	}
	nodes := map[string]Node{}
	ids := map[string]bool{p.ID: true}
	in := map[string]int{}
	out := map[string][]Flow{}
	start := ""
	ends := 0
	for _, n := range p.Nodes {
		if !contracts.ValidID(n.ID) || ids[n.ID] {
			return fmt.Errorf("invalid or duplicate ID %q", n.ID)
		}
		ids[n.ID] = true
		nodes[n.ID] = n
		switch n.Kind {
		case "startEvent":
			if start != "" {
				return fmt.Errorf("exactly one start event required")
			}
			start = n.ID
		case "endEvent":
			ends++
		case "serviceTask":
			if !contracts.ValidID(n.Aisle) || (n.Alternative != "" && (!contracts.ValidID(n.Alternative) || n.Alternative == n.Aisle)) {
				return fmt.Errorf("task %s needs aisle and a distinct optional alternative", n.ID)
			}
		case "exclusiveGateway":
			if !contracts.ValidID(n.Variable) {
				return fmt.Errorf("gateway %s requires af:variable", n.ID)
			}
		default:
			return fmt.Errorf("unsupported node kind %s", n.Kind)
		}
	}
	if start == "" || ends == 0 {
		return fmt.Errorf("start and end events required")
	}
	for _, f := range p.Flows {
		if !contracts.ValidID(f.ID) || ids[f.ID] {
			return fmt.Errorf("invalid or duplicate flow ID %q", f.ID)
		}
		ids[f.ID] = true
		if _, ok := nodes[f.Source]; !ok {
			return fmt.Errorf("dangling source %s", f.Source)
		}
		if _, ok := nodes[f.Target]; !ok {
			return fmt.Errorf("dangling target %s", f.Target)
		}
		in[f.Target]++
		out[f.Source] = append(out[f.Source], f)
		if nodes[f.Source].Kind != "exclusiveGateway" && f.Value != "" {
			return fmt.Errorf("branch value outside gateway")
		}
	}
	for _, n := range p.Nodes {
		switch n.Kind {
		case "startEvent":
			if in[n.ID] != 0 || len(out[n.ID]) != 1 {
				return fmt.Errorf("start requires zero incoming and one outgoing flow")
			}
		case "endEvent":
			if in[n.ID] == 0 || len(out[n.ID]) != 0 {
				return fmt.Errorf("end requires incoming flows and no outgoing flow")
			}
		case "serviceTask":
			if in[n.ID] == 0 || len(out[n.ID]) != 1 {
				return fmt.Errorf("task %s requires incoming flows and one outgoing flow", n.ID)
			}
		case "exclusiveGateway":
			if in[n.ID] == 0 || len(out[n.ID]) < 2 {
				return fmt.Errorf("gateway needs incoming and >=2 outgoing flows")
			}
			values := map[string]bool{}
			for _, f := range out[n.ID] {
				if !contracts.ValidID(f.Value) || values[f.Value] {
					return fmt.Errorf("gateway branch values must be nonempty and unique")
				}
				values[f.Value] = true
			}
		}
	}
	color := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if color[id] == 1 {
			return fmt.Errorf("cycles are unsupported at %s", id)
		}
		if color[id] == 2 {
			return nil
		}
		color[id] = 1
		for _, f := range out[id] {
			if err := visit(f.Target); err != nil {
				return err
			}
		}
		color[id] = 2
		return nil
	}
	if err := visit(start); err != nil {
		return err
	}
	if len(color) != len(nodes) {
		return fmt.Errorf("unreachable nodes")
	}
	return nil
}

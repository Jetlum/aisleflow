package core

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func sample(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../../examples/picking.bpmn")
	require.NoError(t, err)
	return string(b)
}
func TestCompilerDeterministicAndCommitted(t *testing.T) {
	p, err := Parse(strings.NewReader(sample(t)))
	require.NoError(t, err)
	w, s, err := Generate(p, "generated")
	require.NoError(t, err)
	w2, s2, err := Generate(p, "generated")
	require.NoError(t, err)
	require.Equal(t, w, w2)
	require.Equal(t, s, s2)
	committed, err := os.ReadFile("../../temporal/generated/workflow_gen.go")
	require.NoError(t, err)
	require.Equal(t, string(committed), string(w))
	committed, err = os.ReadFile("../../analytics/ent/schema/process_task.go")
	require.NoError(t, err)
	require.Equal(t, string(committed), string(s))
	_, _, err = Generate(p, "bad-package")
	require.Error(t, err)
}
func TestRejectUnsupportedAndMalformedBPMN(t *testing.T) {
	good := sample(t)
	cases := map[string]string{
		"non_executable":    strings.Replace(good, `isExecutable="true"`, `isExecutable="false"`, 1),
		"parallel":          strings.Replace(good, "exclusiveGateway", "parallelGateway", 1),
		"boundary":          strings.Replace(good, "<bpmn:startEvent", `<bpmn:boundaryEvent id="timer"/><bpmn:startEvent`, 1),
		"cycle":             strings.Replace(good, `targetRef="dispatch"`, `targetRef="pick_01"`, 1),
		"dangling":          strings.Replace(good, `targetRef="dispatch"`, `targetRef="missing"`, 1),
		"duplicate":         strings.Replace(good, `id="pick_02"`, `id="pick_01"`, 1),
		"unreachable":       strings.Replace(good, "<bpmn:endEvent", `<bpmn:serviceTask id="orphan" af:aisle="A01"/><bpmn:endEvent`, 1),
		"duplicate_branch":  strings.Replace(good, `af:value="express"`, `af:value="standard"`, 1),
		"foreign_semantics": strings.Replace(good, `af:aisle="A01"`, `implementation="secret" af:aisle="A01"`, 1),
		"wrong_namespace":   strings.ReplaceAll(good, ModelNS, "https://wrong.example"),
		"doctype":           "<!DOCTYPE example>" + good,
		"nested_loop":       strings.Replace(good, `id="pick_01" af:aisle="A01" af:alternative="A02"/>`, `id="pick_01" af:aisle="A01" af:alternative="A02"><bpmn:standardLoopCharacteristics/></bpmn:serviceTask>`, 1),
		"second_root":       good + good,
		"oversize":          string(bytes.Repeat([]byte{' '}, MaxXMLBytes+1)),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) { _, err := Parse(strings.NewReader(input)); require.Error(t, err) })
	}
}
func FuzzParser(f *testing.F) {
	f.Add(`<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"/></definitions>`)
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > MaxXMLBytes+1 {
			t.Skip()
		}
		p, err := Parse(strings.NewReader(s))
		if err == nil {
			require.NoError(t, Validate(p))
		}
	})
}

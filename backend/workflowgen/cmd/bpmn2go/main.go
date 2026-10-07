package main

import (
	"fmt"
	"os"
	"path/filepath"

	"example.com/aisleflow/backend/workflowgen/core"
	"github.com/spf13/cobra"
)

func main() {
	if err := command().Execute(); err != nil {
		os.Exit(1)
	}
}
func command() *cobra.Command {
	var input, output, schema, pkg string
	var check bool
	c := &cobra.Command{Use: "bpmn2go", Short: "Compile the AisleFlow BPMN subset to Go and Ent", SilenceUsage: true, RunE: func(cmd *cobra.Command, _ []string) error {
		f, err := os.Open(input)
		if err != nil {
			return err
		}
		defer f.Close()
		p, err := core.Parse(f)
		if err != nil {
			return err
		}
		w, s, err := core.Generate(p, pkg)
		if err != nil {
			return err
		}
		if filepath.Clean(output) == filepath.Clean(schema) {
			return fmt.Errorf("output paths must differ")
		}
		for _, item := range []struct {
			path string
			data []byte
		}{{output, w}, {schema, s}} {
			if check {
				existing, err := os.ReadFile(item.path)
				if err != nil {
					return err
				}
				if string(existing) != string(item.data) {
					return fmt.Errorf("generated file is stale: %s", item.path)
				}
				continue
			}
			if err := writeFile(item.path, item.data); err != nil {
				return err
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "validated %s: %d nodes, %d flows\n", p.ID, len(p.Nodes), len(p.Flows))
		return nil
	}}
	c.Flags().StringVar(&input, "input", "examples/picking.bpmn", "BPMN XML path")
	c.Flags().StringVar(&output, "output", "backend/temporal/generated/workflow_gen.go", "Go output")
	c.Flags().StringVar(&schema, "schema-output", "backend/analytics/ent/schema/process_task.go", "Ent schema output")
	c.Flags().StringVar(&pkg, "package", "generated", "Go package name")
	c.Flags().BoolVar(&check, "check", false, "fail if committed generated files differ")
	return c
}
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".bpmn2go-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

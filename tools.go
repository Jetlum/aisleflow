//go:build tools

package tools

// Keep the programmatic Ent generator's dependencies in go.mod after go mod tidy.
import _ "entgo.io/ent/entc"

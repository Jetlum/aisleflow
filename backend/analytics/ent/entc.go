//go:build ignore

package main

import (
	"log"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

func main() {
	if err := entc.Generate("./schema", &gen.Config{Target: "./gen", Package: "example.com/aisleflow/backend/analytics/ent/gen"}); err != nil {
		log.Fatal(err)
	}
}

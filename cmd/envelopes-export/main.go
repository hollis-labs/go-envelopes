// Command envelopes-export emits build-time artifacts from the exact
// go-envelopes module version selected by the invoking Go module.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	envelopes "github.com/hollis-labs/go-envelopes"
	"github.com/hollis-labs/go-envelopes/codegen"
)

func main() {
	format := flag.String("format", "catalog", "output format: catalog or typescript")
	output := flag.String("output", "-", "output file, or - for stdout")
	includeUnregistered := flag.Bool("include-unregistered-schemas", false, "include compatibility schemas that are not registered types in TypeScript output")
	flag.Parse()

	registry, err := envelopes.LoadCore(context.Background())
	if err != nil {
		fatal(err)
	}
	catalog, err := registry.ExportCatalog()
	if err != nil {
		fatal(err)
	}

	var content []byte
	switch *format {
	case "catalog":
		content, err = json.MarshalIndent(catalog, "", "  ")
		content = append(content, '\n')
	case "typescript":
		content, err = codegen.TypeScript(catalog, codegen.TypeScriptOptions{
			IncludeUnregisteredSchemas: *includeUnregistered,
		})
	default:
		fatal(fmt.Errorf("unsupported format %q (want catalog or typescript)", *format))
	}
	if err != nil {
		fatal(err)
	}
	if *output == "-" {
		if _, err := os.Stdout.Write(content); err != nil {
			fatal(err)
		}
		return
	}
	if err := os.WriteFile(*output, content, 0o644); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, "envelopes-export:", err)
	os.Exit(1)
}

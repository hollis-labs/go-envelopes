package envelopes_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestExternalConsumer runs a separate Go module from a temp directory. Its
// working directory has no sibling go-envelopes checkout, so all runtime and
// generation assets must come through the imported module API.
func TestExternalConsumer(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repositoryRoot := filepath.Dir(filename)
	consumerDir := t.TempDir()
	resolvedSibling := filepath.Clean(filepath.Join(consumerDir, "..", "..", "libs", "go-envelopes"))
	if _, err := os.Stat(resolvedSibling); !os.IsNotExist(err) {
		t.Fatalf("consumer independence precondition failed: %s exists or stat returned %v", resolvedSibling, err)
	}

	goMod := fmt.Sprintf(`module external-consumer.test

go 1.26.1

require github.com/hollis-labs/go-envelopes v0.0.0

replace github.com/hollis-labs/go-envelopes => %s
`, repositoryRoot)
	pluginManifest := "type: demo.notice\nui:\n  component: cards/Notice\n  export: Notice\n"
	pluginSchema := `{"type":"object","properties":{"message":{"type":"string"}},"required":["message"],"x-plugin-help":"notice"}`
	program := fmt.Sprintf(`package main

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "strings"

    envelopes "github.com/hollis-labs/go-envelopes"
    "github.com/hollis-labs/go-envelopes/codegen"
)

func main() {
    registry, err := envelopes.LoadCore(context.Background())
    if err != nil { panic(err) }
    if err := registry.RegisterTypeFromManifest([]byte(%s), []byte(%s), "demo"); err != nil { panic(err) }
    validationErr := registry.ValidateEnvelope(&envelopes.Envelope{
        V: envelopes.ProtocolVersion, ID: "bad", Type: "demo.notice", Data: map[string]any{},
    })
    var typed *envelopes.ValidationError
    if !errors.As(validationErr, &typed) { panic("structured validation error missing") }
    catalog, err := registry.ExportCatalog()
    if err != nil { panic(err) }
    generated, err := codegen.TypeScript(catalog, codegen.TypeScriptOptions{})
    if err != nil { panic(err) }
    spec, _ := registry.Lookup("demo.notice")
    summary := map[string]any{
        "module": catalog.Source.Module,
        "moduleVersion": catalog.Source.ModuleVersion,
        "manifestDigest": catalog.Source.ManifestDigest,
        "types": len(catalog.Types),
        "schemas": len(catalog.Schemas),
        "failureKeyword": typed.Details()[0].Keyword,
        "pluginAnnotation": spec.DataSchemaDocument.Metadata().Custom["x-plugin-help"],
        "generatedPlugin": strings.Contains(string(generated), "DemoNoticeData"),
    }
    raw, _ := json.Marshal(summary)
    fmt.Println(string(raw))
}
`, strconv.Quote(pluginManifest), strconv.Quote(pluginSchema))

	if err := os.WriteFile(filepath.Join(consumerDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(consumerDir, "main.go"), []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "run", "-mod=mod", ".")
	command.Dir = consumerDir
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("external go run: %v\n%s", err, output)
	}
	var summary struct {
		Module           string `json:"module"`
		ModuleVersion    string `json:"moduleVersion"`
		ManifestDigest   string `json:"manifestDigest"`
		Types            int    `json:"types"`
		Schemas          int    `json:"schemas"`
		FailureKeyword   string `json:"failureKeyword"`
		PluginAnnotation string `json:"pluginAnnotation"`
		GeneratedPlugin  bool   `json:"generatedPlugin"`
	}
	if err := json.Unmarshal(output, &summary); err != nil {
		t.Fatalf("decode consumer output: %v\n%s", err, output)
	}
	if summary.Module != "github.com/hollis-labs/go-envelopes" || summary.ModuleVersion != "v0.0.0" {
		t.Fatalf("consumer source identity = %q@%q", summary.Module, summary.ModuleVersion)
	}
	if !strings.HasPrefix(summary.ManifestDigest, "sha256:") || summary.Types < 2 || summary.Schemas < 2 {
		t.Fatalf("consumer catalog summary = %#v", summary)
	}
	if summary.FailureKeyword != "required" || summary.PluginAnnotation != "notice" || !summary.GeneratedPlugin {
		t.Fatalf("consumer contract summary = %#v", summary)
	}

	command = exec.Command("go", "run", "-mod=mod",
		"github.com/hollis-labs/go-envelopes/cmd/envelopes-export",
		"-format", "typescript")
	command.Dir = consumerDir
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err = command.CombinedOutput()
	if err != nil {
		t.Fatalf("external module-owned generator: %v\n%s", err, output)
	}
	generated := string(output)
	if !strings.Contains(generated, "go-envelopes@v0.0.0") || !strings.Contains(generated, "export interface InfoCardData") {
		t.Fatalf("external generator lacked source identity or types:\n%s", generated)
	}
}

package envelopes_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
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
	var pluginType *envelopes.CatalogType
	for index := range catalog.Types {
		if catalog.Types[index].Name == "demo.notice" { pluginType = &catalog.Types[index]; break }
	}
	var pluginSchema *envelopes.SchemaResource
	if pluginType != nil {
		for index := range catalog.Schemas {
			if catalog.Schemas[index].URI == pluginType.SchemaURI { pluginSchema = &catalog.Schemas[index]; break }
		}
	}
	pluginCatalog := pluginType != nil && pluginType.Source == "plugin" && pluginType.PluginID == "demo" &&
		pluginType.Annotations != nil && pluginType.Annotations.Custom["x-plugin-help"] == "notice"
	pluginDocument := false
	if pluginSchema != nil {
		var document map[string]any
		if err := json.Unmarshal(pluginSchema.Document, &document); err != nil { panic(err) }
		properties, _ := document["properties"].(map[string]any)
		message, _ := properties["message"].(map[string]any)
		pluginDocument = pluginSchema.Registered && pluginSchema.Source == "plugin" &&
			pluginSchema.PluginID == "demo" && message["type"] == "string"
	}
    summary := map[string]any{
        "module": catalog.Source.Module,
        "moduleVersion": catalog.Source.ModuleVersion,
        "manifestDigest": catalog.Source.ManifestDigest,
        "types": len(catalog.Types),
        "schemas": len(catalog.Schemas),
        "failureKeyword": typed.Details()[0].Keyword,
        "pluginAnnotation": spec.DataSchemaDocument.Metadata().Custom["x-plugin-help"],
		"pluginCatalog": pluginCatalog,
		"pluginDocument": pluginDocument,
		"generatedField": strings.Contains(string(generated), "message: string;"),
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
	command.Env = commandEnvironment(map[string]string{"GOWORK": "off"})
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
		PluginCatalog    bool   `json:"pluginCatalog"`
		PluginDocument   bool   `json:"pluginDocument"`
		GeneratedField   bool   `json:"generatedField"`
	}
	if err := json.Unmarshal(output, &summary); err != nil {
		t.Fatalf("decode consumer output: %v\n%s", err, output)
	}
	if summary.Module != "github.com/hollis-labs/go-envelopes" || summary.ModuleVersion != "(devel; local replacement)" {
		t.Fatalf("consumer source identity = %q@%q", summary.Module, summary.ModuleVersion)
	}
	if !strings.HasPrefix(summary.ManifestDigest, "sha256:") || summary.Types < 2 || summary.Schemas < 2 {
		t.Fatalf("consumer catalog summary = %#v", summary)
	}
	if summary.FailureKeyword != "required" || summary.PluginAnnotation != "notice" ||
		!summary.PluginCatalog || !summary.PluginDocument || !summary.GeneratedField {
		t.Fatalf("consumer contract summary = %#v", summary)
	}

	command = exec.Command("go", "run", "-mod=mod",
		"github.com/hollis-labs/go-envelopes/cmd/envelopes-export",
		"-format", "typescript")
	command.Dir = consumerDir
	command.Env = commandEnvironment(map[string]string{"GOWORK": "off"})
	output, err = command.CombinedOutput()
	if err != nil {
		t.Fatalf("external module-owned generator: %v\n%s", err, output)
	}
	generated := string(output)
	if !strings.Contains(generated, "go-envelopes@(devel; local replacement)") ||
		!strings.Contains(generated, "export interface InfoCardData") || strings.Contains(generated, repositoryRoot) {
		t.Fatalf("external generator lacked source identity or types:\n%s", generated)
	}
}

func TestExternalConsumerOrdinarySelectedModuleIdentity(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repositoryRoot := filepath.Dir(filename)
	proxyURL, version := writeModuleProxy(t, repositoryRoot)
	consumerDir := t.TempDir()
	goMod := fmt.Sprintf(`module selected-consumer.test

go 1.26.1

require github.com/hollis-labs/go-envelopes %s
`, version)
	program := `package main

import (
    "context"
    "encoding/json"
    "fmt"

    envelopes "github.com/hollis-labs/go-envelopes"
)

func main() {
    registry, err := envelopes.LoadCore(context.Background())
    if err != nil { panic(err) }
    catalog, err := registry.ExportCatalog()
    if err != nil { panic(err) }
    raw, _ := json.Marshal(catalog.Source)
    fmt.Println(string(raw))
}
`
	if err := os.WriteFile(filepath.Join(consumerDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(consumerDir, "main.go"), []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "run", "-mod=mod", ".")
	command.Dir = consumerDir
	command.Env = commandEnvironment(map[string]string{
		"GOWORK":    "off",
		"GOSUMDB":   "off",
		"GOPRIVATE": "none",
		"GONOPROXY": "none",
		"GONOSUMDB": "none",
		"GOPROXY":   proxyURL,
	})
	output, err := command.CombinedOutput()
	if err != nil {
		envCommand := exec.Command("go", "env", "GOPROXY", "GOPRIVATE", "GONOPROXY", "GONOSUMDB")
		envCommand.Dir = consumerDir
		envCommand.Env = command.Env
		envOutput, _ := envCommand.CombinedOutput()
		t.Fatalf("selected-module go run: %v\nproxy=%s\ngo env:\n%s\n%s", err, proxyURL, envOutput, output)
	}
	var source struct {
		Module        string `json:"module"`
		ModuleVersion string `json:"moduleVersion"`
	}
	jsonStart := bytes.IndexByte(output, '{')
	if jsonStart < 0 {
		t.Fatalf("source identity output contained no JSON: %s", output)
	}
	if err := json.Unmarshal(output[jsonStart:], &source); err != nil {
		t.Fatalf("decode source identity: %v\n%s", err, output)
	}
	if source.Module != "github.com/hollis-labs/go-envelopes" || source.ModuleVersion != version {
		t.Fatalf("ordinary selected module identity = %#v", source)
	}
}

type moduleFile struct {
	relative string
	data     []byte
}

func writeModuleProxy(t *testing.T, repositoryRoot string) (string, string) {
	t.Helper()
	const modulePath = "github.com/hollis-labs/go-envelopes"
	files := make([]moduleFile, 0)
	digest := sha256.New()
	err := filepath.Walk(repositoryRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(repositoryRoot, path)
		if err != nil {
			return err
		}
		if relative == ".git" {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read module file %s: %w", relative, err)
		}
		_, _ = digest.Write([]byte(filepath.ToSlash(relative)))
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write(data)
		files = append(files, moduleFile{relative: relative, data: data})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	version := fmt.Sprintf("v0.0.0-test.%x", digest.Sum(nil)[:6])
	proxyRoot := t.TempDir()
	versionDir := filepath.Join(proxyRoot, filepath.FromSlash(modulePath), "@v")
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	goMod, err := os.ReadFile(filepath.Join(repositoryRoot, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, version+".mod"), goMod, 0o644); err != nil {
		t.Fatal(err)
	}
	info := fmt.Sprintf(`{"Version":%q,"Time":"2026-09-04T00:00:00Z"}`, version)
	if err := os.WriteFile(filepath.Join(versionDir, version+".info"), []byte(info), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "list"), []byte(version+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	archive, err := os.Create(filepath.Join(versionDir, version+".zip"))
	if err != nil {
		t.Fatal(err)
	}
	zipWriter := zip.NewWriter(archive)
	for _, file := range files {
		entry, err := zipWriter.Create(modulePath + "@" + version + "/" + filepath.ToSlash(file.relative))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(file.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return "file://" + filepath.ToSlash(proxyRoot), version
}

func commandEnvironment(overrides map[string]string) []string {
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, item := range os.Environ() {
		name, _, _ := strings.Cut(item, "=")
		if _, overridden := overrides[name]; overridden {
			continue
		}
		environment = append(environment, item)
	}
	for name, value := range overrides {
		environment = append(environment, name+"="+value)
	}
	return environment
}

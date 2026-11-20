package internal_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	modulePath   = "save_pwd"
	corePrefix   = modulePath + "/internal/"
	adapterRoot  = modulePath + "/internal/adapters"
	adaptersDir  = "adapters"
	repoRootDir  = ".."
	internalRoot = "."
)

// forbiddenStdlib lists standard-library packages the core must not touch:
// I/O with the outside world and non-cryptographic or unsafe primitives
// belong in adapters.
func forbiddenStdlib(path string) bool {
	switch path {
	case "os", "os/exec", "net", "math/rand", "math/rand/v2", "unsafe":
		return true
	}
	return strings.HasPrefix(path, "net/")
}

// isStdlib relies on the Go convention that only standard-library import
// paths lack a dot in their first element. The module itself ("save_pwd")
// also has no dot, so it is checked before this is called.
func isStdlib(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".")
}

// coreImportViolation explains why a core package may not import path, or
// returns "" when the import is allowed.
func coreImportViolation(path string) string {
	switch {
	case path == adapterRoot || strings.HasPrefix(path, adapterRoot+"/"):
		return "core must not import an adapter"
	case strings.HasPrefix(path, corePrefix):
		return ""
	case path == modulePath || strings.HasPrefix(path, modulePath+"/"):
		return "core may only import other core packages of this module"
	case !isStdlib(path):
		return "core must not import a non-stdlib module"
	case forbiddenStdlib(path):
		return "core must not import this stdlib package"
	}
	return ""
}

func TestCoreImportRule(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "errors", want: true},
		{path: "crypto/rand", want: true},
		{path: "save_pwd/internal/domain", want: true},
		{path: "os", want: false},
		{path: "os/exec", want: false},
		{path: "net", want: false},
		{path: "net/http", want: false},
		{path: "math/rand", want: false},
		{path: "math/rand/v2", want: false},
		{path: "unsafe", want: false},
		{path: "github.com/spf13/cobra", want: false},
		{path: "golang.org/x/crypto/argon2", want: false},
		{path: "save_pwd/internal/adapters/cli", want: false},
		{path: "save_pwd/cmd/save_pwd", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := coreImportViolation(tt.path) == ""
			if got != tt.want {
				t.Fatalf("allowed(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestCorePackagesRespectImportRule(t *testing.T) {
	for _, file := range goFiles(t, internalRoot) {
		if isUnderAdapters(file) {
			continue
		}
		f := parseFile(t, file, parser.ImportsOnly)
		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("%s: bad import %s: %v", file, imp.Path.Value, err)
			}
			if reason := coreImportViolation(path); reason != "" {
				t.Errorf("%s imports %q: %s", file, path, reason)
			}
		}
	}
}

// Dependencies are wired explicitly in main, so no package may rely on init().
func TestNoInitFunctions(t *testing.T) {
	for _, file := range goFiles(t, repoRootDir) {
		f := parseFile(t, file, parser.SkipObjectResolution)
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Recv == nil && fn.Name.Name == "init" {
				t.Errorf("%s declares func init()", file)
			}
		}
	}
}

func goFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != root && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return files
}

func isUnderAdapters(file string) bool {
	first, _, _ := strings.Cut(filepath.ToSlash(filepath.Clean(file)), "/")
	return first == adaptersDir
}

func parseFile(t *testing.T, file string, mode parser.Mode) *ast.File {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, mode)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	return f
}

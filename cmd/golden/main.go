// cmd/golden writes the expected outputs golden_test.go compares the wrapper against.
// It clones TA-Lib at the embedded version, builds the official static library with
// CMake, and runs c/dump.c against it: every function, default optional inputs.
//
// Usage: go run ./cmd/golden [-out dir] [-ref git-ref]
// (from the module root; default testdata/golden, at the tag of TALibVersion)
//
// Needs git, CMake and a C compiler ($CC, else cc). Unix only.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
)

func main() {
	out := flag.String("out", filepath.Join("testdata", "golden"), "output directory")
	ref := flag.String("ref", "", "TA-Lib branch or tag to build instead of the embedded version's tag")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/golden [-out dir] [-ref git-ref]")
		os.Exit(2)
	}
	if err := run(*out, *ref); err != nil {
		fmt.Fprintln(os.Stderr, "cmd/golden:", err)
		os.Exit(1)
	}
}

func run(outDir, version string) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return fmt.Errorf("run from the module root: %v", err)
	}
	if version == "" {
		if version, err = taLibVersion(filepath.Join(root, "version_gen.go")); err != nil {
			return err
		}
	}
	dumpSrc := filepath.Join(root, "cmd", "golden", "c", "dump.c")
	if _, err := os.Stat(dumpSrc); err != nil {
		return err
	}
	cc := os.Getenv("CC")
	if cc == "" {
		cc = "cc"
	}

	tmp, err := os.MkdirTemp("", "ta-lib-golden-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	src := filepath.Join(tmp, "ta-lib")
	build := filepath.Join(tmp, "build")
	stage := filepath.Join(tmp, "out")
	dump := filepath.Join(tmp, "dump")
	fmt.Printf("cmd/golden: TA-Lib %s, CC=%s\n", version, cc)

	if err := runCmd(nil, "git", "clone", "--quiet", "--depth=1", "--branch", version,
		"https://github.com/TA-Lib/ta-lib", src); err != nil {
		return err
	}
	// Upstream's own configuration: CMake reads the compiler from $CC.
	ccEnv := []string{"CC=" + cc}
	if err := runCmd(ccEnv, "cmake", "-S", src, "-B", build,
		"-DBUILD_SHARED_LIBS=OFF", "-DBUILD_STATIC_LIBS=ON",
		"-DBUILD_DEV_TOOLS=OFF", "-DBUILD_BENCHMARKS=OFF"); err != nil {
		return err
	}
	if err := runCmd(ccEnv, "cmake", "--build", build, "--target", "ta-lib-static",
		"--parallel", strconv.Itoa(runtime.NumCPU())); err != nil {
		return err
	}
	lib := filepath.Join(build, "libta-lib.a")
	if _, err := os.Stat(lib); err != nil {
		return fmt.Errorf("official static library not found: %v", err)
	}

	if err := runCmd(ccEnv, "sh", "-c", `exec $CC -o "$1" "$2" -I"$3" "$4" -lm`, "sh",
		dump, dumpSrc, filepath.Join(src, "include"), lib); err != nil {
		return err
	}
	if err := os.Mkdir(stage, 0o755); err != nil {
		return err
	}
	if err := runCmd(nil, dump, stage); err != nil {
		return err
	}
	return replaceCSVs(stage, outDir)
}

// taLibVersion reads TALibVersion from version_gen.go, which cmd/gen writes from its
// go:generate argument.
func taLibVersion(path string) (string, error) {
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return "", err
	}
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, name := range vs.Names {
				if name.Name != "TALibVersion" || i >= len(vs.Values) {
					continue
				}
				if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if v, err := strconv.Unquote(lit.Value); err == nil && v != "" {
						return "v" + v, nil
					}
				}
			}
		}
	}
	return "", fmt.Errorf("%s: no TALibVersion string constant", path)
}

// replaceCSVs swaps the *.csv files of outDir for those of stage, leaving any other file
// alone. outDir is only touched once every golden has been written.
func replaceCSVs(stage, outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	old, err := filepath.Glob(filepath.Join(outDir, "*.csv"))
	if err != nil {
		return err
	}
	for _, p := range old {
		if err := os.Remove(p); err != nil {
			return err
		}
	}
	files, err := filepath.Glob(filepath.Join(stage, "*.csv"))
	if err != nil {
		return err
	}
	for _, p := range files {
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outDir, filepath.Base(p)), data, 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("cmd/golden: wrote %d files to %s\n", len(files), outDir)
	return nil
}

func runCmd(env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), env...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %v: %v", name, args, err)
	}
	return nil
}

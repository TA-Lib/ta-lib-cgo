package ta_test

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The goldens are written by cmd/golden from the official TA-Lib build and the table
// (golden_gen_test.go) by cmd/gen.
//
//	TALIB_GOLDEN_DIR    read the goldens from this directory instead of testdata/golden
//	TALIB_GOLDEN_EXACT  =1: compare bit patterns. Valid only against goldens built on
//	                    this machine; libm differs in the last bits between machines.

// goldenInput holds the columns of input.csv.
type goldenInput struct {
	open, high, low, close, volume, real1, periods []float64
}

// goldenFunc calls one wrapper with the default optional inputs. Each element of the
// result is a []float64 or a []int32, in the order of outputs.
type goldenFunc struct {
	name    string
	outputs []string
	call    func(in *goldenInput) []any
}

func goldenDir() string {
	if d := os.Getenv("TALIB_GOLDEN_DIR"); d != "" {
		return d
	}
	return filepath.Join("testdata", "golden")
}

func readCSV(t *testing.T, path string) (header []string, rows [][]string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	recs, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if len(recs) == 0 {
		t.Fatalf("%s: empty", path)
	}
	return recs[0], recs[1:]
}

func loadGoldenInput(t *testing.T, dir string) *goldenInput {
	t.Helper()
	path := filepath.Join(dir, "input.csv")
	header, rows := readCSV(t, path)
	want := []string{"open", "high", "low", "close", "volume", "real1", "periods"}
	if !slices.Equal(header, want) {
		t.Fatalf("%s: header %v, want %v", path, header, want)
	}
	cols := make([][]float64, len(want))
	for i, rec := range rows {
		for c, s := range rec {
			v, err := strconv.ParseFloat(s, 64)
			if err != nil {
				t.Fatalf("%s: row %d: %v", path, i+1, err)
			}
			cols[c] = append(cols[c], v)
		}
	}
	return &goldenInput{cols[0], cols[1], cols[2], cols[3], cols[4], cols[5], cols[6]}
}

func floatMatch(got, want float64, exact bool) bool {
	switch {
	case math.IsNaN(got) || math.IsNaN(want):
		return math.IsNaN(got) && math.IsNaN(want)
	case exact:
		return math.Float64bits(got) == math.Float64bits(want)
	case math.IsInf(got, 0) || math.IsInf(want, 0):
		return got == want
	default:
		return math.Abs(got-want) <= 1e-9*math.Max(1, math.Abs(want))
	}
}

func TestGolden(t *testing.T) {
	dir := goldenDir()
	exact := os.Getenv("TALIB_GOLDEN_EXACT") == "1"
	in := loadGoldenInput(t, dir)
	n := len(in.close)
	if n == 0 {
		t.Fatalf("%s: no rows", filepath.Join(dir, "input.csv"))
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.csv"))
	if err != nil {
		t.Fatal(err)
	}
	inTable := map[string]bool{}
	for _, g := range goldenFuncs {
		inTable[g.name] = true
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".csv")
		if name != "input" && !inTable[name] {
			t.Errorf("%s has no table entry", f)
		}
	}

	for _, g := range goldenFuncs {
		t.Run(g.name, func(t *testing.T) {
			path := filepath.Join(dir, g.name+".csv")
			header, rows := readCSV(t, path)
			if !slices.Equal(header, g.outputs) {
				t.Fatalf("%s: columns %v, want %v", path, header, g.outputs)
			}
			if len(rows) != n {
				t.Fatalf("%s: %d rows, want %d", path, len(rows), n)
			}
			outs := g.call(in)
			if len(outs) != len(g.outputs) {
				t.Fatalf("call returned %d outputs, want %d", len(outs), len(g.outputs))
			}

			mismatches := 0
			var first string
			for i, rec := range rows {
				for c, out := range outs {
					var ok bool
					var got, want any
					switch out := out.(type) {
					case []float64:
						w, err := strconv.ParseFloat(rec[c], 64)
						if err != nil {
							t.Fatalf("%s: row %d: %v", path, i+1, err)
						}
						ok, got, want = floatMatch(out[i], w, exact), out[i], w
					case []int32:
						w, err := strconv.ParseInt(rec[c], 10, 32)
						if err != nil {
							t.Fatalf("%s: row %d: %v", path, i+1, err)
						}
						ok, got, want = out[i] == int32(w), out[i], int32(w)
					default:
						t.Fatalf("output %d has type %T", c, out)
					}
					if !ok {
						if mismatches == 0 {
							verb := "%d"
							if _, isFloat := got.(float64); isFloat {
								verb = "%.17g"
							}
							first = fmt.Sprintf("bar %d, %s: got "+verb+", want "+verb, i, g.outputs[c], got, want)
						}
						mismatches++
					}
				}
			}
			if mismatches > 0 {
				t.Errorf("%d mismatching values; first at %s", mismatches, first)
			}
		})
	}
}

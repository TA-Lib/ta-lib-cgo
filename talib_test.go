package ta_test

import (
	"math"
	"slices"
	"strconv"
	"testing"

	ta "github.com/TA-Lib/ta-lib-cgo"
)

// prices is a synthetic input with enough bars to exercise all lookback periods.
var prices = func() []float64 {
	s := make([]float64, 200)
	for i := range s {
		// Simple sine-wave-like series to avoid constant input edge cases.
		s[i] = 100.0 + 10.0*math.Sin(float64(i)*0.1)
	}
	return s
}()

// ohlcv returns parallel OHLCV slices of the same length as prices.
func ohlcv(n int) (high, low, open, closePrice, volume []float64) {
	high = make([]float64, n)
	low = make([]float64, n)
	open = make([]float64, n)
	closePrice = make([]float64, n)
	volume = make([]float64, n)
	for i := range high {
		base := 100.0 + 10.0*math.Sin(float64(i)*0.1)
		high[i] = base + 1.0
		low[i] = base - 1.0
		open[i] = base + 0.3
		closePrice[i] = base - 0.3
		volume[i] = 1e6 + float64(i)*1000
	}
	return
}

func isNaN(f float64) bool { return math.IsNaN(f) }

// assertShapeInvariants checks that out:
//   - has the same length as in
//   - out[:lookback] are all NaN
//   - out[lookback:] are all finite (not NaN, not Inf)
func assertShapeInvariants(t *testing.T, name string, in, out []float64, lookback int) {
	t.Helper()
	if len(out) != len(in) {
		t.Errorf("%s: output length %d != input length %d", name, len(out), len(in))
		return
	}
	for i := 0; i < lookback && i < len(out); i++ {
		if !isNaN(out[i]) {
			t.Errorf("%s: out[%d] = %v, want NaN (lookback prefix)", name, i, out[i])
		}
	}
	for i := lookback; i < len(out); i++ {
		if isNaN(out[i]) || math.IsInf(out[i], 0) {
			t.Errorf("%s: out[%d] = %v, want finite value", name, i, out[i])
		}
	}
}

// --- Checkpoint 1a: shape invariants for common single-output functions ---

func TestSmaShape(t *testing.T) {
	period := 10
	out := ta.Sma(prices, period, nil)
	lb := ta.SmaLookback(period)
	assertShapeInvariants(t, "Sma", prices, out, lb)
}

func TestEmaShape(t *testing.T) {
	period := 10
	out := ta.Ema(prices, period, nil)
	lb := ta.EmaLookback(period)
	assertShapeInvariants(t, "Ema", prices, out, lb)
}

func TestRsiShape(t *testing.T) {
	period := 14
	out := ta.Rsi(prices, period, nil)
	lb := ta.RsiLookback(period)
	assertShapeInvariants(t, "Rsi", prices, out, lb)
}

func TestAtrShape(t *testing.T) {
	high, low, _, closePrice, _ := ohlcv(len(prices))
	period := 14
	out := ta.Atr(high, low, closePrice, period, nil)
	lb := ta.AtrLookback(period)
	assertShapeInvariants(t, "Atr", high, out, lb)
}

func TestAdxShape(t *testing.T) {
	high, low, _, closePrice, _ := ohlcv(len(prices))
	period := 14
	out := ta.Adx(high, low, closePrice, period, nil)
	lb := ta.AdxLookback(period)
	assertShapeInvariants(t, "Adx", high, out, lb)
}

// --- Checkpoint 1b: shape invariants for multi-output functions ---

func TestMacdShape(t *testing.T) {
	macd, signal, hist := ta.Macd(prices, 12, 26, 9, nil, nil, nil)
	lb := ta.MacdLookback(12, 26, 9)
	assertShapeInvariants(t, "Macd.macd", prices, macd, lb)
	assertShapeInvariants(t, "Macd.signal", prices, signal, lb)
	assertShapeInvariants(t, "Macd.hist", prices, hist, lb)
}

func TestBbandsShape(t *testing.T) {
	upper, middle, lower := ta.Bbands(prices, 20, 2.0, 2.0, 0, nil, nil, nil)
	lb := ta.BbandsLookback(20, 2.0, 2.0, 0)
	assertShapeInvariants(t, "Bbands.upper", prices, upper, lb)
	assertShapeInvariants(t, "Bbands.middle", prices, middle, lb)
	assertShapeInvariants(t, "Bbands.lower", prices, lower, lb)
}

func TestAroonShape(t *testing.T) {
	high, low, _, _, _ := ohlcv(len(prices))
	period := 14
	down, up := ta.Aroon(high, low, period, nil, nil)
	lb := ta.AroonLookback(period)
	assertShapeInvariants(t, "Aroon.down", high, down, lb)
	assertShapeInvariants(t, "Aroon.up", high, up, lb)
}

// --- Checkpoint 1c: outBuf reuse ---

func TestOutBufReuse(t *testing.T) {
	buf := make([]float64, len(prices))
	out := ta.Sma(prices, 10, buf)
	// Must return the same backing array.
	if &out[0] != &buf[0] {
		t.Error("Sma: outBuf with sufficient cap should be reused, but got a new allocation")
	}
}

// An outBuf sharing memory with the input must not be written to; one that only touches it
// is still reused. Offsets index one backing array of 3n elements.
func TestOutBufOverlappingInput(t *testing.T) {
	n := len(prices)
	want := ta.Sma(prices, 10, nil)
	for _, tc := range []struct {
		name                  string
		inOff, bufOff, bufLen int
		reuse                 bool
	}{
		{"buffer is the input", n, n, n, false},
		{"buffer starts inside the input", n, n + 2, n, false},
		{"buffer ends inside the input", n, 2, n, false},
		{"only the capacity reaches the input", n, n - 1, 0, false},
		{"buffer ends where the input starts", n, 0, 0, true},
		{"buffer starts where the input ends", n, 2 * n, 0, true},
	} {
		back := make([]float64, 3*n)
		in := back[tc.inOff : tc.inOff+n]
		copy(in, prices)
		out := ta.Sma(in, 10, back[tc.bufOff:tc.bufOff+tc.bufLen])
		if !slices.EqualFunc(out, want, func(a, b float64) bool { return a == b || isNaN(a) && isNaN(b) }) {
			t.Errorf("%s: output differs from Sma with a nil outBuf", tc.name)
		}
		if !slices.Equal(in, prices) {
			t.Errorf("%s: input was modified", tc.name)
		}
		if reused := &out[0] == &back[tc.bufOff]; reused != tc.reuse {
			t.Errorf("%s: outBuf reused = %v, want %v", tc.name, reused, tc.reuse)
		}
	}
}

func TestOutBufReallocWhenTooSmall(t *testing.T) {
	small := make([]float64, 1) // cap < len(prices)
	out := ta.Sma(prices, 10, small)
	if &out[0] == &small[0] {
		t.Error("Sma: outBuf with insufficient cap should trigger new allocation")
	}
	if len(out) != len(prices) {
		t.Errorf("Sma: reallocated output has length %d, want %d", len(out), len(prices))
	}
}

// --- Checkpoint 1d: panic on bad input ---

func TestPanicOnEmptyInput(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Sma(empty): expected panic, got none")
		}
		err, ok := r.(*ta.TALibError)
		if !ok {
			t.Fatalf("Sma(empty): panic value is %T, want *ta.TALibError", r)
		}
		if err.RetCode != 2 {
			t.Errorf("Sma(empty): RetCode = %d, want 2 (TA_BAD_PARAM)", err.RetCode)
		}
	}()
	ta.Sma([]float64{}, 10, nil)
}

// assertBadParamPanic checks that f panics with a *ta.TALibError carrying TA_BAD_PARAM,
// not a Go runtime error.
func assertBadParamPanic(t *testing.T, name string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		if r == nil {
			t.Fatalf("%s: expected panic, got none", name)
		}
		err, ok := r.(*ta.TALibError)
		if !ok {
			t.Fatalf("%s: panic value is %T (%v), want *ta.TALibError", name, r, r)
		}
		if err.RetCode != 2 {
			t.Errorf("%s: RetCode = %d, want 2 (TA_BAD_PARAM)", name, err.RetCode)
		}
	}()
	f()
}

func TestPanicOnInvalidPeriod(t *testing.T) {
	assertBadParamPanic(t, "Sma(period=MinInt32)", func() { ta.Sma(prices, math.MinInt32, nil) })
	assertBadParamPanic(t, "Sma(period=0)", func() { ta.Sma(prices, 0, nil) })
	assertBadParamPanic(t, "Sma(period=-5)", func() { ta.Sma(prices, -5, nil) })
}

func TestPanicOnInvalidMAType(t *testing.T) {
	assertBadParamPanic(t, "Ma(maType=99)", func() { ta.Ma(prices, 10, 99, nil) })
	assertBadParamPanic(t, "Bbands(maType=99)", func() { ta.Bbands(prices, 20, 2.0, 2.0, 99, nil, nil, nil) })
}

func TestPanicOnIntBeyondCInt(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("every int fits in a C int")
	}
	var p32, p31 int64 = 1<<32 + 3, 1 << 31
	assertBadParamPanic(t, "Sma(period=1<<32+3)", func() { ta.Sma(prices, int(p32), nil) })
	assertBadParamPanic(t, "Ma(maType=1<<32)", func() { ta.Ma(prices, 10, int(p32-3), nil) })
	if got := ta.SmaLookback(int(p31)); got != -1 {
		t.Errorf("SmaLookback(1<<31) = %d, want -1", got)
	}
}

func TestPanicOnMismatchedInputLengths(t *testing.T) {
	high, low, _, closePrice, _ := ohlcv(len(prices))
	assertBadParamPanic(t, "Atr(short low)", func() { ta.Atr(high, low[:len(low)-1], closePrice, 14, nil) })
	assertBadParamPanic(t, "Atr(long close)", func() { ta.Atr(high, low, append(closePrice, 1), 14, nil) })
}

// --- Checkpoint 1e: series no longer than the lookback ---

func TestShortSeriesAllNaN(t *testing.T) {
	period := 10
	lb := ta.SmaLookback(period)
	for _, n := range []int{1, lb - 1, lb} {
		in := prices[:n]
		out := ta.Sma(in, period, nil)
		if len(out) != n {
			t.Errorf("Sma(n=%d, period=%d): output length %d, want %d", n, period, len(out), n)
			continue
		}
		for i, v := range out {
			if !isNaN(v) {
				t.Errorf("Sma(n=%d, period=%d): out[%d] = %v, want NaN", n, period, i, v)
			}
		}
	}
}

func TestShortSeriesReusesOutBuf(t *testing.T) {
	buf := make([]float64, 5)
	for i := range buf {
		buf[i] = 42
	}
	out := ta.Sma(prices[:5], 10, buf)
	if &out[0] != &buf[0] {
		t.Error("Sma(short): outBuf with sufficient cap should be reused, but got a new allocation")
	}
	for i, v := range out {
		if !isNaN(v) {
			t.Errorf("Sma(short): out[%d] = %v, want NaN", i, v)
		}
	}
}

func TestShortSeriesInt32Zero(t *testing.T) {
	lb := ta.CdldojiLookback()
	if lb < 1 {
		t.Fatalf("CdldojiLookback() = %d, want >= 1 for this test", lb)
	}
	high, low, open, closePrice, _ := ohlcv(lb)
	for _, n := range []int{1, lb} {
		out := ta.Cdldoji(open[:n], high[:n], low[:n], closePrice[:n], []int32{7, 7, 7})
		if len(out) != n {
			t.Errorf("Cdldoji(n=%d): output length %d, want %d", n, len(out), n)
			continue
		}
		for i, v := range out {
			if v != 0 {
				t.Errorf("Cdldoji(n=%d): out[%d] = %d, want 0", n, i, v)
			}
		}
	}
}

// --- Checkpoint 1f: lookback sanity ---

func TestSmaLookback(t *testing.T) {
	// SMA(n) lookback is n-1 for n >= 2 (period=1 is invalid in TA-Lib).
	for _, tc := range []struct{ period, want int }{
		{2, 1},
		{3, 2},
		{10, 9},
		{20, 19},
	} {
		got := ta.SmaLookback(tc.period)
		if got != tc.want {
			t.Errorf("SmaLookback(%d) = %d, want %d", tc.period, got, tc.want)
		}
	}
}

// --- Checkpoint 1g: known SMA values (hand-verifiable) ---

func TestSmaKnownValues(t *testing.T) {
	in := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	out := ta.Sma(in, 3, nil)

	if len(out) != 10 {
		t.Fatalf("len(out) = %d, want 10", len(out))
	}

	// First 2 values (lookback = 2) must be NaN.
	for i := 0; i < 2; i++ {
		if !isNaN(out[i]) {
			t.Errorf("out[%d] = %v, want NaN", i, out[i])
		}
	}

	// SMA(3): average of 3 consecutive values.
	expected := []float64{2, 3, 4, 5, 6, 7, 8, 9}
	for i, want := range expected {
		got := out[i+2]
		if math.Abs(got-want) > 1e-10 {
			t.Errorf("out[%d] = %v, want %v", i+2, got, want)
		}
	}
}

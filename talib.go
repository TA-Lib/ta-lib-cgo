package ta

/*
#cgo CFLAGS: -I${SRCDIR}/include
#cgo LDFLAGS: -lm
#include "ta_libc.h"
#include "ta_func.h"
*/
import "C"
import (
	"math"
	"unsafe"
)

//go:generate go run ./cmd/gen v0.8.3
//go:generate go run ./cmd/golden

func init() {
	if rc := C.TA_Initialize(); rc != C.TA_SUCCESS {
		panic(&TALibError{RetCode: int(rc), Message: retCodeMessage(int(rc))})
	}
}

// reuseOrAlloc returns outBuf[:n] if cap(outBuf) >= n and outBuf shares no memory with
// any of ins, else allocates a new slice. The wrapper writes the output before C has
// read all of the inputs.
func reuseOrAlloc(outBuf []float64, n int, ins ...[]float64) []float64 {
	if cap(outBuf) >= n && !overlaps(outBuf[:n], ins) {
		return outBuf[:n]
	}
	return make([]float64, n)
}

// overlaps reports whether out shares memory with any of ins.
func overlaps(out []float64, ins [][]float64) bool {
	if len(out) == 0 {
		return false
	}
	lo := uintptr(unsafe.Pointer(&out[0]))
	hi := lo + uintptr(len(out))*unsafe.Sizeof(out[0])
	for _, in := range ins {
		if len(in) == 0 {
			continue
		}
		p := uintptr(unsafe.Pointer(&in[0]))
		if p < hi && lo < p+uintptr(len(in))*unsafe.Sizeof(in[0]) {
			return true
		}
	}
	return false
}

// fillNaN fills s with NaN values.
func fillNaN(s []float64) {
	nan := math.NaN()
	for i := range s {
		s[i] = nan
	}
}

// inPtr returns a C pointer to the first element of a float64 slice.
func inPtr(s []float64) *C.double {
	return (*C.double)(unsafe.Pointer(&s[0]))
}

// inPtrInt returns a C pointer to the first element of an int32 slice.
func inPtrInt(s []int32) *C.int {
	return (*C.int)(unsafe.Pointer(&s[0]))
}

// reuseOrAllocInt32 is reuseOrAlloc for int32 slices.
func reuseOrAllocInt32(outBuf []int32, n int) []int32 {
	if cap(outBuf) >= n {
		return outBuf[:n]
	}
	return make([]int32, n)
}

// fillNaNInt32 zeroes s (int32 has no NaN; use 0 as sentinel).
func fillNaNInt32(s []int32) {
	for i := range s {
		s[i] = 0
	}
}

// cInt converts v to a C int, panicking with TA_BAD_PARAM when it does not fit.
func cInt(v int) C.int {
	if !fitsCInt(v) {
		panic(&TALibError{RetCode: 2, Message: retCodeMessage(2)})
	}
	return C.int(v)
}

// fitsCInt reports whether v can be passed as a C int. INT_MIN is excluded: TA-Lib
// reads it as "use the default", which no out-of-range Go value should select.
func fitsCInt(v int) bool {
	return int(C.int(v)) == v && v != math.MinInt32
}

func checkRC(rc C.TA_RetCode) {
	if rc != C.TA_SUCCESS {
		code := int(rc)
		panic(&TALibError{RetCode: code, Message: retCodeMessage(code)})
	}
}

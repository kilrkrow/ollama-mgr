//go:build windows

package edge

import "testing"

func TestHresultFailed(t *testing.T) {
	cases := map[uintptr]bool{
		0:                  false, // S_OK
		1:                  false, // S_FALSE
		0x800401F0:         true,  // CO_E_NOTINITIALIZED, zero-extended as delivered on amd64
		0x80010106:         true,  // RPC_E_CHANGED_MODE
		0x80070002:         true,  // HRESULT_FROM_WIN32(ERROR_FILE_NOT_FOUND)
		0xFFFFFFFF80004005: true,  // E_FAIL, sign-extended
	}
	for in, want := range cases {
		if got := hresultFailed(in); got != want {
			t.Errorf("hresultFailed(0x%X) = %v, want %v", in, got, want)
		}
	}
}

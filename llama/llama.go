package llama

/*
#cgo CFLAGS: -I${SRCDIR}/../llama.cpp/include -I${SRCDIR}/../llama.cpp/ggml/include -I${SRCDIR}/../build/ggml/src -I${SRCDIR}/../build/ggml/include -I${SRCDIR}/../build/common -I${SRCDIR}/../cbridge
#cgo LDFLAGS: ${SRCDIR}/../cbridge/schema_bridge.o ${SRCDIR}/../build/src/libllama.a ${SRCDIR}/../build/ggml/src/libggml.a ${SRCDIR}/../build/ggml/src/libggml-base.a ${SRCDIR}/../build/ggml/src/libggml-cpu.a ${SRCDIR}/../build/common/libllama-common.a ${SRCDIR}/../build/common/libllama-common-base.a
#cgo linux LDFLAGS: -lstdc++ -lm -lpthread -ldl
#cgo darwin LDFLAGS: -lc++ -lm
#cgo windows LDFLAGS: -lstdc++ -lm -lwinpthread
#include "wrapper.h"
#include "schema_bridge.h"
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type Model struct {
	ptr *C.glean_model_t
}

func SetLogLevel(level int) {
	C.glean_set_log_level(C.int32_t(level))
}

func Load(modelPath string, nCtx int, nThreads int, nGPULayers int) (*Model, error) {
	cPath := C.CString(modelPath)
	defer C.free(unsafe.Pointer(cPath))

	m := C.glean_load(cPath, C.int32_t(nCtx), C.int32_t(nThreads), C.int32_t(nGPULayers))
	if m == nil {
		return nil, fmt.Errorf("failed to load model from %s", modelPath)
	}
	return &Model{ptr: m}, nil
}

func (m *Model) Free() {
	if m.ptr != nil {
		C.glean_free(m.ptr)
		m.ptr = nil
	}
}

func (m *Model) Tokenize(text string, addBOS bool, parseSpecial bool) ([]int32, error) {
	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	cap := 8192
	tokens := make([]int32, cap)
	n := C.glean_tokenize(m.ptr, cText, (*C.int32_t)(unsafe.Pointer(&tokens[0])), C.int32_t(cap), C.bool(addBOS), C.bool(parseSpecial))
	if n < 0 {
		needed := -int32(n)
		tokens = make([]int32, needed)
		n = C.glean_tokenize(m.ptr, cText, (*C.int32_t)(unsafe.Pointer(&tokens[0])), C.int32_t(needed), C.bool(addBOS), C.bool(parseSpecial))
		if n < 0 {
			return nil, fmt.Errorf("tokenization failed")
		}
	}
	return tokens[:n], nil
}

func (m *Model) TokenToPiece(token int32) string {
	buf := make([]byte, 256)
	n := C.glean_token_to_piece(m.ptr, C.int32_t(token), (*C.char)(unsafe.Pointer(&buf[0])), C.int32_t(len(buf)))
	if n > 0 {
		return string(buf[:n])
	}
	if n < 0 {
		buf = make([]byte, -n)
		n = C.glean_token_to_piece(m.ptr, C.int32_t(token), (*C.char)(unsafe.Pointer(&buf[0])), C.int32_t(len(buf)))
		if n > 0 {
			return string(buf[:n])
		}
	}
	return ""
}

func (m *Model) Decode(tokens []int32) error {
	if len(tokens) == 0 {
		return nil
	}
	n := C.glean_decode(m.ptr, (*C.int32_t)(unsafe.Pointer(&tokens[0])), C.int32_t(len(tokens)))
	if n != 0 {
		return fmt.Errorf("decode failed with code %d", n)
	}
	return nil
}

func (m *Model) SampleNext() int32 {
	C.jsonify_synchronize(m.ptr)
	return int32(C.jsonify_sample_next(m.ptr))
}

func (m *Model) ClearContext() {
	C.jsonify_clear_context(m.ptr)
}

func (m *Model) AcceptToken(token int32) {
	C.glean_accept_token(m.ptr, C.int32_t(token))
}

func (m *Model) NVocab() int32 {
	return int32(C.glean_n_vocab(m.ptr))
}

func (m *Model) TokenEOS() int32 {
	return int32(C.glean_token_eos(m.ptr))
}

func (m *Model) SetGrammar(grammarStr string, grammarRoot string) error {
	cGrammar := C.CString(grammarStr)
	defer C.free(unsafe.Pointer(cGrammar))
	cRoot := C.CString(grammarRoot)
	defer C.free(unsafe.Pointer(cRoot))
	ret := C.glean_set_grammar(m.ptr, cGrammar, cRoot)
	if ret != 0 {
		return fmt.Errorf("failed to initialize grammar sampler")
	}
	return nil
}

func (m *Model) ClearGrammar() {
	C.glean_clear_grammar(m.ptr)
}

// DecodeAndSample is a convenience method that decodes tokens and samples the next token.
func (m *Model) DecodeAndSample(tokens []int32) (int32, error) {
	err := m.Decode(tokens)
	if err != nil {
		return 0, err
	}
	token := m.SampleNext()
	m.AcceptToken(token)
	return token, nil
}

// SchemaToGrammar converts a JSON Schema string to GBNF grammar using llama.cpp's native converter.
func SchemaToGrammar(jsonSchema string) (string, error) {
	cSchema := C.CString(jsonSchema)
	defer C.free(unsafe.Pointer(cSchema))

	var cErr *C.char
	result := C.glean_schema_to_grammar(cSchema, &cErr)
	if result == nil {
		if cErr != nil {
			errMsg := C.GoString(cErr)
			C.free(unsafe.Pointer(cErr))
			return "", fmt.Errorf("%s", errMsg)
		}
		return "", fmt.Errorf("schema conversion failed with no error message")
	}
	defer C.free(unsafe.Pointer(result))

	return C.GoString(result), nil
}

// ChatApplyTemplate formats a prompt using the model's built-in chat template.
// Pass systemMsg and/or userMsg. addAss controls whether assistant prefix is appended.
func (m *Model) ChatApplyTemplate(systemMsg, userMsg string, addAss bool) (string, error) {
	var cSys *C.char
	if systemMsg != "" {
		cSys = C.CString(systemMsg)
		defer C.free(unsafe.Pointer(cSys))
	}
	var cUser *C.char
	if userMsg != "" {
		cUser = C.CString(userMsg)
		defer C.free(unsafe.Pointer(cUser))
	}

	result := C.glean_chat_apply_template(m.ptr, cSys, cUser, C.bool(addAss))
	if result == nil {
		return "", fmt.Errorf("chat template application failed")
	}
	defer C.free(unsafe.Pointer(result))

	return C.GoString(result), nil
}

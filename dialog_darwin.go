package main

/*
#include <stdlib.h>
#include "agent_darwin.h"
*/
import "C"

import "unsafe"

// ask shows a question with two buttons and is true for the first.
func ask(title, text, yes, no string) bool {
	values := []*C.char{C.CString(title), C.CString(text), C.CString(yes), C.CString(no)}
	defer func() {
		for _, value := range values {
			C.free(unsafe.Pointer(value))
		}
	}()
	return C.fmAsk(values[0], values[1], values[2], values[3]) == 1
}

// tell shows a message and waits for OK.
func tell(title, text string) {
	first, second := C.CString(title), C.CString(text)
	defer C.free(unsafe.Pointer(first))
	defer C.free(unsafe.Pointer(second))
	C.fmTell(first, second)
}

package main

/*
#include <stdlib.h>
#include "agent_darwin.h"
*/
import "C"

import (
	"sync"
	"time"
	"unsafe"
)

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

// windowButtons says that choose shows the buttons it is given.
const windowButtons = true

// What the button that leaves the window of choose open does, while that window is there.
var staying struct {
	sync.Mutex
	action func() string
}

// choose shows a message with the buttons given, the first being the one of Return, and answers
// the place of the one clicked, from 0, or -1 when the window was closed without a click.
func choose(title, text string, buttons ...string) int {
	return chooseOpen(title, text, buttons, -1, nil)
}

// chooseOpen is choose with a button, the one at the place stay, that does not close the window:
// a click turns the buttons off, runs action, and turns them on again, with the title action
// answers for that button.
func chooseOpen(title, text string, buttons []string, stay int, action func() string) int {
	first, second := C.CString(title), C.CString(text)
	defer C.free(unsafe.Pointer(first))
	defer C.free(unsafe.Pointer(second))
	list := (**C.char)(C.malloc(C.size_t(len(buttons)) * C.size_t(unsafe.Sizeof((*C.char)(nil)))))
	defer C.free(unsafe.Pointer(list))
	for place, button := range buttons {
		value := C.CString(button)
		defer C.free(unsafe.Pointer(value))
		unsafe.Slice(list, len(buttons))[place] = value
	}
	staying.Lock()
	staying.action = action
	staying.Unlock()
	return int(C.fmChoose(first, second, list, C.int(len(buttons)), C.int(stay)))
}

//export goChooseStay
func goChooseStay() {
	staying.Lock()
	action := staying.action
	staying.Unlock()
	if action == nil {
		return
	}
	// in its own thread: this is the main one, and the window has to keep answering
	go func() {
		title := C.CString(action())
		defer C.free(unsafe.Pointer(title))
		C.fmChooseDone(title)
	}()
}

// closeWindows closes the windows that are open, without a click. On the Mac quitting with a
// window open closes that window in the place of the agent, which then never quits.
func closeWindows() {
	for try := 0; try < 20 && C.fmCloseWindow() == 1; try++ {
		time.Sleep(50 * time.Millisecond)
	}
}

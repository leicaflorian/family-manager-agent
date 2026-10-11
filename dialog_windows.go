package main

import "golang.org/x/sys/windows"

const (
	boxYesNo      = 0x00000004
	boxQuestion   = 0x00000020
	boxWarning    = 0x00000030
	boxForeground = 0x00010000
	boxTopmost    = 0x00040000
	answerYes     = 6
)

// ask shows a question with Sì and No (Windows writes the buttons in its own language) and is true for yes.
func ask(title, text, yes, no string) bool {
	caption, _ := windows.UTF16PtrFromString(title)
	message, _ := windows.UTF16PtrFromString(text)
	answer, _ := windows.MessageBox(0, message, caption, boxYesNo|boxQuestion|boxForeground|boxTopmost)
	return answer == answerYes
}

// tell shows a message and waits for OK.
func tell(title, text string) {
	caption, _ := windows.UTF16PtrFromString(title)
	message, _ := windows.UTF16PtrFromString(text)
	windows.MessageBox(0, message, caption, boxWarning|boxForeground|boxTopmost)
}

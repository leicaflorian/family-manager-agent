package main

import "golang.org/x/sys/windows"

const (
	boxYesNo      = 0x00000004
	boxQuestion   = 0x00000020
	boxWarning    = 0x00000030
	boxInfo       = 0x00000040
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

// windowButtons says that choose shows only OK: a message of Windows has no buttons with a name of
// ours, so what the Mac puts in the window of Informazioni stays in the menu here.
const windowButtons = false

// choose shows a message that is not a warning and waits for OK: the buttons are not shown, and
// the answer is always the first.
func choose(title, text string, buttons ...string) int {
	caption, _ := windows.UTF16PtrFromString(title)
	message, _ := windows.UTF16PtrFromString(text)
	windows.MessageBox(0, message, caption, boxInfo|boxForeground|boxTopmost)
	return 0
}

// chooseOpen is choose here: no button is shown, so none leaves the window open.
func chooseOpen(title, text string, buttons []string, stay int, action func() string) int {
	return choose(title, text, buttons...)
}

// closeWindows does nothing here: a message of Windows is not closed from outside.
func closeWindows() {}

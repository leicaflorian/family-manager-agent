package main

// install does nothing on the Mac: the link familymanageragent://open is declared by Info.plist
// (CFBundleURLTypes), and macOS learns it when the app is opened the first time. It is never true:
// nothing was installed by this start.
func install() bool { return false }

// tellInstalled has nothing to say here.
func tellInstalled() {}

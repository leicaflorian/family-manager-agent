package main

// install does nothing on the Mac: the link familymanageragent://open is declared by Info.plist
// (CFBundleURLTypes), and macOS learns it when the app is opened the first time.
func install() {}

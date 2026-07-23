//go:build !darwin

package main

func setTrayApp(*App)  {}
func startSystemTray() {}
func stopSystemTray()  {}

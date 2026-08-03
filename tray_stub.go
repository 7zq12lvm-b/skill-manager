//go:build !darwin

package main

import skillmgr "skill-manager/internal"

func setTrayApp(*App)                              {}
func startSystemTray()                             {}
func stopSystemTray()                              {}
func updateSystemTrayInventory(skillmgr.Inventory) {}

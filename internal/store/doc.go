// Package store keeps the app data in the data folder (see paths.DataDir):
// imported VPN configurations, registered apps and preferences
// (settings.json). Passwords are protected with DPAPI on Windows and with an
// AES-GCM key file elsewhere.
package store

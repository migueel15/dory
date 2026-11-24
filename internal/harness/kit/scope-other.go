//go:build !linux

package kit

func UserScope() string { return "" }

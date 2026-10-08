//go:build !darwin

package main

import (
	"encoding/json"
	"errors"
	"unsafe"
)

type nativePluginPage struct{}

func createNativePluginPage(unsafe.Pointer, string, PluginPageBounds, func(json.RawMessage)) (*nativePluginPage, error) {
	return nil, errors.New("plugin carrier: platform has no qualified native carrier")
}
func (*nativePluginPage) close()                    {}
func (*nativePluginPage) send(string)               {}
func (*nativePluginPage) position(PluginPageBounds) {}

package plugin

import (
	"errors"
	"testing"
)

func TestPortableDeviceAliasesCannotBecomeResourcesOrDataDirectories(t *testing.T) {
	for _, name := range []string{"COM¹", "com².log", "COM³.txt", "LPT¹", "lpt².log", "LPT³.txt"} {
		t.Run(name, func(t *testing.T) {
			if ValidResourcePath("nested/" + name) {
				t.Error("portable resource path admitted a device alias")
			}
			server := Server{Name: serverName("local"), Transport: stdio, Command: "fixture", Dir: "${PLUGIN_DATA}/nested/" + name}
			if _, err := NewRelease(testDigest("1"), Declaration{Name: "local", Servers: []Server{server}}); !errors.Is(err, ErrInvalid) {
				t.Fatalf("device working directory error = %v", err)
			}
		})
	}
	for _, name := range []string{"COM10", "COM¹notes", "LPT³notes", "LPT0.txt"} {
		if !ValidResourcePath("nested/" + name) {
			t.Errorf("rejected ordinary resource %q", name)
		}
	}
}

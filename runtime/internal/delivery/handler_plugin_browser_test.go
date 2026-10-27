package delivery

import (
	"bytes"
	json "encoding/json/v2"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/control"
)

// Populate every actual source field, including fields added by a future Qimu
// change. A forgotten delivery projection must fail before publishing that wire.
func populateBrowserProjection(value reflect.Value, path string) {
	switch value.Kind() {
	case reflect.String:
		if vocabulary, ok := value.Interface().(interface{ Values() []string }); ok {
			value.SetString(vocabulary.Values()[0])
		} else {
			value.SetString(qimu.Hash([]byte(path)).String())
		}
	case reflect.Struct:
		for i := range value.NumField() {
			populateBrowserProjection(value.Field(i), path+"."+value.Type().Field(i).Name)
		}
	case reflect.Slice:
		value.Set(reflect.MakeSlice(value.Type(), 2, 2))
		for i := range value.Len() {
			populateBrowserProjection(value.Index(i), path+string(rune('0'+i)))
		}
	default:
		panic("browser wire fixture requires an explicit new kind: " + value.Kind().String())
	}
}

func TestBrowserDeliveryProjectionPreservesTheCompleteApprovedUse(t *testing.T) {
	var original control.UseGrant
	populateBrowserProjection(reflect.ValueOf(&original).Elem(), "grant")
	want, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(wireBrowserUse(original))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("public Use projection differs from the original approval\noriginal: %s\npublic:   %s", want, got)
	}
	if browserRealm(wireBrowserRealm(original.Realm)) != original.Realm {
		t.Fatal("Realm projection changed the actual execution identity")
	}
}

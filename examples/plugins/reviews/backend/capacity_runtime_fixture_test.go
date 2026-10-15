package reviews

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/jsoninput"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestReceiptCapacityRuntimeFixture(t *testing.T) {
	directory := os.Getenv("FLAME_TEST_REVIEWS_CAPACITY_DATA")
	if directory == "" {
		return
	}
	store := openTestStore(t, directory)
	initial, err := store.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	input := Update{ID: "capacity-missing", ExpectedRevision: 1, Status: Open}
	admitted, err := jsoninput.Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	// Prepare through the business owner before publishing the HTTP endpoint;
	// filling the real bound is setup, not part of Runtime's handshake window.
	for index := range MaxReceipts {
		result, err := store.Update(t.Context(), fmt.Sprintf("runtime-capacity-%d", index), input, admitted)
		if err != nil || result.Type != NotFound {
			t.Fatalf("prepare receipt capacity: %+v, %v", result, err)
		}
	}
	server, err := NewServer(store)
	if err != nil {
		t.Fatal(err)
	}
	remote := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, nil))
	defer remote.Close()
	fmt.Println(remote.URL)
	if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
		t.Fatal(err)
	}
	current, err := store.List(t.Context())
	if err != nil || !reflect.DeepEqual(current, initial) {
		t.Fatalf("capacity refusal changed private review: %+v, %v", current, err)
	}
	var receipts int
	if err := store.db.QueryRowContext(t.Context(), `SELECT count(*) FROM invocations`).Scan(&receipts); err != nil || receipts != MaxReceipts {
		t.Fatalf("capacity refusal changed retained receipts: %d, %v", receipts, err)
	}
}

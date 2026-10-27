package main

import (
	"flag"
	"fmt"
	"os"
	"reflect"

	"github.com/Tangerg/flame/qimu/control"
	controlhost "github.com/Tangerg/flame/qimu/control/host"
	"github.com/Tangerg/flame/qimu/examples/document"
)

func main() {
	output := flag.String("out", "web/src/api.generated.ts", "generated domain DTO path")
	flag.Parse()
	file, err := os.Create(*output)
	if err == nil {
		err = control.WriteTypeScriptTypes(file, reflect.TypeFor[document.TaskView](), reflect.TypeFor[document.PreviewMetadata](), reflect.TypeFor[controlhost.Credentials]())
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

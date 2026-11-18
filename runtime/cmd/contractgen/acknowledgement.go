package main

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/packages/go-sdk/wiregen"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func writeGoAcknowledgementCheck(body *strings.Builder, meta delivery.MethodMeta) {
	fmt.Fprintf(body, "\nfunc acknowledgementMismatch%s(parameters, value any) string {\n", meta.GoName)
	fmt.Fprintf(body, "request := parameters.(%s)\nresult := value.(%s)\n", goType(meta.Params), goType(meta.Result))
	body.WriteString("switch {\n")
	for _, match := range meta.Acknowledgement {
		request, _ := protocol.WireFieldNamed(meta.Params, match.Request)
		result, _ := protocol.WireFieldNamed(meta.Result, match.Result)
		wanted, got := "request."+request.GoName, "result."+result.GoName
		if match.Compare == delivery.AcknowledgementPresence {
			if request.Type.Kind() == reflect.Slice {
				wanted = "(len(" + wanted + ") != 0)"
			}
			got = "(" + got + " != nil)"
		}
		fmt.Fprintf(body, "case %s != %s: return %q\n", wanted, got, result.Name)
	}
	body.WriteString("default: return \"\"\n}\n}\n\n")
}

func (m *methodsEmitter) acknowledgementChecks(metas []delivery.MethodMeta) {
	m.line("/** Capture only the request evidence needed when its result arrives. */")
	m.line("export function wireAcknowledgementCheck(")
	m.line("  method: WireMethodName,")
	m.line("  params: unknown,")
	m.line("): ((result: unknown) => string | undefined) | undefined {")
	m.line("  switch (method) {")
	for _, meta := range metas {
		if len(meta.Acknowledgement) == 0 {
			continue
		}
		name := wiregen.StringLiteral(meta.Name.String())
		m.line("    case %s: {", name)
		m.line("      const request = params as WireParams<%s>;", name)
		m.line("      const expected = {")
		for _, match := range meta.Acknowledgement {
			wanted := "request[" + wiregen.StringLiteral(match.Request) + "]"
			if match.Compare == delivery.AcknowledgementPresence {
				request, _ := protocol.WireFieldNamed(meta.Params, match.Request)
				if request.Type.Kind() == reflect.Slice {
					wanted = "(" + wanted + "?.length ?? 0) !== 0"
				} else {
					wanted += " === true"
				}
			}
			m.line("        %s: %s,", wiregen.StringLiteral(match.Result), wanted)
		}
		m.line("      };")
		m.line("      return (value) => {")
		m.line("        const result = value as WireResult<%s>;", name)
		for _, match := range meta.Acknowledgement {
			field := wiregen.StringLiteral(match.Result)
			got := "result[" + field + "]"
			if match.Compare == delivery.AcknowledgementPresence {
				got = "(" + got + " !== undefined)"
			}
			m.line("        if (%s !== expected[%s]) return %s;", got, field, field)
		}
		m.line("        return undefined;")
		m.line("      };")
		m.line("    }")
	}
	m.line("    default:")
	m.line("      return undefined;")
	m.line("  }")
	m.line("}")
	m.line("")
}

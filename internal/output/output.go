// Package output renders command results in token-efficient formats.
package output

import (
	"fmt"
	"io"
	"reflect"
)

// Concise is implemented by item types that can render a one-line summary.
type Concise interface {
	Concise() string
}

// Emit writes items to w in the given format: concise, json, jsonl, table.
// items must be a slice.
func Emit(w io.Writer, format string, items any) error {
	v := reflect.ValueOf(items)
	if v.Kind() != reflect.Slice {
		return fmt.Errorf("output: items must be a slice, got %T", items)
	}
	switch format {
	case "concise", "":
		return emitConcise(w, v)
	case "json":
		return emitJSON(w, items)
	case "jsonl":
		return emitJSONL(w, v)
	case "table":
		return emitTable(w, v)
	default:
		return fmt.Errorf("output: unknown format %q (want concise|json|jsonl|table)", format)
	}
}

func emitConcise(w io.Writer, v reflect.Value) error {
	for i := 0; i < v.Len(); i++ {
		c, ok := v.Index(i).Interface().(Concise)
		if !ok {
			return fmt.Errorf("output: %s does not implement Concise", v.Index(i).Type())
		}
		if _, err := fmt.Fprintln(w, c.Concise()); err != nil {
			return err
		}
	}
	return nil
}

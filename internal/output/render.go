package output

import (
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"text/tabwriter"
)

func emitJSON(w io.Writer, items any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}

func emitJSONL(w io.Writer, v reflect.Value) error {
	enc := json.NewEncoder(w)
	for i := 0; i < v.Len(); i++ {
		if err := enc.Encode(v.Index(i).Interface()); err != nil {
			return err
		}
	}
	return nil
}

// emitTable reflects over the element struct's json tags for column headers.
func emitTable(w io.Writer, v reflect.Value) error {
	if v.Len() == 0 {
		return nil
	}
	elemType := v.Index(0).Type()
	var cols []string
	for i := 0; i < elemType.NumField(); i++ {
		tag := elemType.Field(i).Tag.Get("json")
		name := strings.Split(tag, ",")[0]
		if name == "" || name == "-" {
			name = elemType.Field(i).Name
		}
		cols = append(cols, name)
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	headers := make([]string, len(cols))
	for i, c := range cols {
		headers[i] = strings.ToUpper(c)
	}
	io.WriteString(tw, strings.Join(headers, "\t")+"\n")
	for i := 0; i < v.Len(); i++ {
		row := v.Index(i)
		cells := make([]string, elemType.NumField())
		for j := 0; j < elemType.NumField(); j++ {
			cells[j] = toCell(row.Field(j))
		}
		io.WriteString(tw, strings.Join(cells, "\t")+"\n")
	}
	return tw.Flush()
}

func toCell(fv reflect.Value) string {
	switch fv.Kind() {
	case reflect.String:
		return fv.String()
	default:
		b, _ := json.Marshal(fv.Interface())
		return string(b)
	}
}

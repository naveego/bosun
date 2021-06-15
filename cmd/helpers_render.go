package cmd

import (
	"bytes"
	"encoding/json"
	"github.com/jedib0t/go-pretty/table"
	"github.com/jedib0t/go-pretty/text"
	"github.com/naveego/bosun/pkg/util"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
	"os"
	"sort"
	"strings"
)

// renderOutput is an alias for printOutput because I can't remember which it is
func renderOutput(out interface{}, columns ...string) error {
	return printOutput(out, columns...)
}

func printOutput(out interface{}, columns ...string) error {
	return printOutputWithDefaultFormat("t", out, columns...)
}

func printOutputWithDefaultFormat(defaultFormat string, out interface{}, columns ...string) error {

	format := viper.GetString(ArgGlobalOutput)

	if format == "" {
		format = defaultFormat
	}

	formatKey := strings.ToLower(format[0:1])

	switch formatKey {
	case "j":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	case "y":
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(out)
	case "t":

		var columnConfigs []table.ColumnConfig
		var header table.Row
		var rows []table.Row

		t := table.NewWriter()

		switch ot := out.(type) {
		case util.Tabler:
			headerCols := ot.Headers()
			for _, col := range headerCols {
				header = append(header, col)
				columnConfigs = append(columnConfigs, table.ColumnConfig{
					Name:   col,
					Align:  text.AlignLeft,
					VAlign: text.VAlignTop,
				})
			}

			rowCols := ot.Rows()
			for _, cols := range rowCols {
				row := table.Row{}
				for _, col := range cols {
					row = append(row, col)
				}
				rows = append(rows, row)
			}

		default:
			segs := strings.Split(format, "=")
			if len(segs) > 1 {
				columns = strings.Split(segs[1], ",")
			}
			j, err := json.Marshal(out)
			if err != nil {
				return err
			}
			var mapSlice []map[string]json.RawMessage
			err = json.Unmarshal(j, &mapSlice)
			if err != nil {
				return errors.Wrapf(err, "only slices of structs or maps can be rendered as a table, but got %T", out)
			}
			if len(mapSlice) == 0 {
				return nil
			}

			first := mapSlice[0]

			var keys []string
			if len(columns) > 0 {
				keys = columns
			} else {
				for k := range first {
					keys = append(keys, k)
				}
				sort.Strings(keys)
			}
			for _, col := range keys {
				header = append(header, col)
				columnConfigs = append(columnConfigs, table.ColumnConfig{
					Name:   col,
					Align:  text.AlignLeft,
					VAlign: text.VAlignTop,
				})
			}
			for _, m := range mapSlice {
				var row table.Row

				for _, k := range keys {
					if v, ok := m[k]; ok && len(v) > 0 {
						var value string
						if bytes.HasPrefix(v, []byte("\"")) {
							_ = json.Unmarshal(v, &value)
						} else {
							value = string(v)
						}
						row = append(row, value)
					} else {
						row = append(row, "")
					}
				}
				rows = append(rows, row)
			}
		}

		t.AppendHeader(header)
		t.SetColumnConfigs(columnConfigs)
		t.SetOutputMirror(os.Stdout)
		t.AppendRows(rows)

		t.Render()

		return nil
	default:
		return errors.Errorf("Unrecognized format %q (valid formats are 'json', 'yaml', and 'table')", format)
	}

}

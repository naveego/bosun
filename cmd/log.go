// Copyright © 2018 NAME HERE <EMAIL ADDRESS>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/fatih/color"
	"github.com/rs/xid"
	"github.com/spf13/cobra"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var logCmd = addCommand(rootCmd, &cobra.Command{
	Use:     "log",
	Aliases: []string{"logs"},
	Short:   "Commands for working with logs.",
})

var logParseXids = addCommand(logCmd, &cobra.Command{
	Use:     "humanize [log]",
	Aliases: []string{},
	Short:   "Takes text from or reads from stdin with various readability improvements.",
	RunE: func(cmd *cobra.Command, args []string) error {

		xidRE := regexp.MustCompile("[a-v0-9]{20}")
		epochRE := regexp.MustCompile("[0-9]{10,13}")
		colorRE := regexp.MustCompile("\\033" + `\[[0-9;]+m`)
		var scanner *bufio.Scanner
		if len(args) > 0 {
			content := strings.Join(args, " ")
			b := bytes.NewBufferString(content)
			scanner = bufio.NewScanner(b)
		} else {
			scanner = bufio.NewScanner(os.Stdin)
		}

		for scanner.Scan() {
			text := scanner.Text()

			// add timestamp to xids
			parsed := xidRE.ReplaceAllStringFunc(string(text), func(s string) string {
				parsedXID, err := xid.FromString(s)
				if err != nil {
					return s
				}
				return fmt.Sprintf("%s(%s)", s, parsedXID.Time().Local().Format(time.RFC3339))
			})
			// strip colors
			parsed = colorRE.ReplaceAllString(parsed, "")
			// convert epoch to utc
			parsed = epochRE.ReplaceAllStringFunc(parsed, func(s string) string {
				epoch, err := strconv.Atoi(s)
				if err != nil {
					return s
				}
				switch len(s) {
				case 10:
				case 13:
					epoch = epoch / 1000
				}

				d := time.Unix(int64(epoch), 0)
				return d.Local().Format(time.RFC3339)
			})

			fmt.Println(parsed)
		}

		return nil
	},
})

var logParseJson = addCommand(logCmd, &cobra.Command{
	Use:     "unjson [log]",
	Aliases: []string{},
	Short:   "Reformats JSON logs into something easier to read",
	RunE: func(cmd *cobra.Command, args []string) error {

		var scanner *bufio.Scanner

		if len(args) > 0 {
			content := strings.Join(args, " ")
			b := bytes.NewBufferString(content)
			scanner = bufio.NewScanner(b)
		} else {
			scanner = bufio.NewScanner(os.Stdin)
		}

		var err error
		var data map[string]interface{}
		for scanner.Scan() {

			text := scanner.Text()
			err = json.Unmarshal([]byte(text), &data)

			if err != nil{
				fmt.Printf("%s %s\n",color.BlueString("NOT JSON: "), text)
				continue
			}

			if len(data) == 0 {
				continue
			}

			timestamp := "unknown"
			category := "unknown"
			levelString := "unknown"
			message := "unknown"
			var level float64
			var ok bool

			singleLineKeys := make([]string, 0, len(data))
			multilineKeys := make([]string, 0, len(data))
			for k, v := range data {
				if k == "timestamp" {
					timestamp = v.(string)
				} else if k == "level" {
					if level, ok = v.(float64); !ok {
						levelString = "UNKN"
					} else {
						switch level {
						case 3:
							levelString = "FAIL"
						case 4:
							levelString = "WARN"
						case 5:
							levelString = "INFO"
						case 6:
							levelString = "DBUG"
						default:
							levelString = fmt.Sprintf("%30f", level)
						}
					}
				} else if k == "category" {
					category = v.(string)
				} else if k == "message" {
					message = v.(string)
				} else {
					var s string
					if s, ok = v.(string); ok {
						if strings.Count(s, "\n") > 0 {
							multilineKeys = append(multilineKeys, k)
						} else {
							singleLineKeys = append(singleLineKeys, k)
						}
					} else {
						singleLineKeys = append(singleLineKeys, k)
					}
				}
			}

			if true {
				switch level {
				case 3:

					levelString = color.RedString(levelString)
				case 4:
					levelString = color.YellowString(levelString)
				case 5:
					levelString = color.BlueString(levelString)
				case 6:
					levelString = color.GreenString(levelString)
				}
			}

			sort.Strings(singleLineKeys)
			sort.Strings(multilineKeys)

			_, _ = fmt.Printf("%s %s %s : ", levelString, timestamp, category)

			message = " " + strings.ReplaceAll(message, "\\n", "\n ")
			_, _ = fmt.Fprintln(os.Stdout, message)

			for _, k := range singleLineKeys {
				_, _ = fmt.Fprintf(os.Stdout, "%s=%v; ", k, data[k])
			}
			for _, k := range multilineKeys {
				s := strings.ReplaceAll(data[k].(string), "\\n", "\n ")
				_, _ = fmt.Fprintf(os.Stdout, "\n%s=%s; ", k, s)
			}
			_, _ = fmt.Fprintln(os.Stdout)

			data = nil

		}

		return nil
	},
})

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
	"fmt"
	"github.com/rs/xid"
	"github.com/spf13/cobra"
	"os"
	"regexp"
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
	Use:   "humanize [log]",
	Aliases:[]string{},
	Short: "Takes text from or reads from stdin with various readability improvements.",
	RunE: func(cmd *cobra.Command, args []string) error {

		xidRE := regexp.MustCompile("[a-v0-9]{20}")
		epochRE := regexp.MustCompile("[0-9]{10,13}")
		colorRE := regexp.MustCompile("\\033" +`\[[0-9;]+m`)
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
				switch len(s){
				case 10:
				case 13:
					epoch = epoch / 1000
				}

				d := time.Unix( int64(epoch), 0)
				return d.Local().Format(time.RFC3339)
			})

			fmt.Println(parsed)
		}

		return nil
	},
})
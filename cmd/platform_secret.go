package cmd

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/jedib0t/go-pretty/list"

)

var platformSecretCmd = addCommand(platformCmd, &cobra.Command{
	Use:          "secret",
	Short:        "Secret management commands.",
	Aliases:[]string{"secrets"},
})

var platformSecretListCmd = addCommand(platformSecretCmd, &cobra.Command{
	Use:          "list",
	Aliases:[]string{"ls"},
	Short:        "Lists secrets",
	RunE: func(cmd *cobra.Command, args []string) error {
		b, p := MustGetPlatform()
		envs, err := p.GetEnvironmentConfigs()
		check(err)


		for _, env := range envs {
			color.Blue("%s:", env.Name)
			for _, secret := range env.Secrets.Names {
				fmt.Printf("- %s\n", secret)
			}
		}

		return nil
	},
})
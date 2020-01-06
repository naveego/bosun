package cmd

import (
	"errors"
	"fmt"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/vcs"

	// "github.com/pkg/errors"
	"github.com/spf13/cobra"
	// "os"
)

type MoveMap struct {
	Moves map[string]string `yaml:"moves"`
}

// This is intended to be a command for merging changes from the new UI back to the old UI if doing it manually gets too annoying.
var gitMoveMergeCmd = addCommand(gitCmd, &cobra.Command{
	Use:    "move-merge {map-file} {from} {to}",
	Args:   cobra.ExactArgs(3),
	Hidden: true,
	Short:  "Moves files which have been committed in one branch, then merges them into another branch.",
	Long:   `Requires github hub tool to be installed (https://hub.github.com/).`,
	RunE: func(cmd *cobra.Command, args []string) error {

		b := MustGetBosun()
		ctx := b.NewContext()
		var err error
		repoDir, err := git.GetCurrentRepoPath()
		if err != nil {
			return err
		}
		g, err := git.NewGitWrapper(repoDir)
		if err != nil {
			return err
		}
		repo := &vcs.LocalRepo{Path: repoDir}
		check(g.Pull())

		mapFilePath := args[0]
		fromBranch := args[1]
		toBranch := args[2]

		fmt.Printf("Mapping files using %q and merging from branch %q to %q.", mapFilePath, fromBranch, toBranch)

		tmpBranch := fmt.Sprintf("tmp/move-merge")
		g.Exec("branch", "-d", tmpBranch)

		check(repo.SwitchToNewBranch(ctx, fromBranch, tmpBranch))

		return errors.New("not implemented")
	},
})

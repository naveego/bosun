package cmd

// var appUICmd = addCommand(appCmd, &cobra.Command{
// 	Use:          "ui",
// 	Short:        "UI for managing apps.",
// 	SilenceUsage: true,
// 	RunE: func(cmd *cobra.Command, args []string) error {
// 		viper.BindPFlags(cmd.Flags())
// 		viper.SetDefault(ArgFilteringAll, true)
//
// 		b := MustGetBosun()
//
// 		apps := b.GetApps()
//
// 		wd, _ := os.Getwd()
//
// 		ctx := b.NewContext()
//
// 		t := tabby.New()
// 		t.AddHeader("APP", "CLONED", "VERSION", "REPO", "PATH", "BRANCH")
// 		for _, app := range apps {
// 			var isCloned, repo, path, branch, version string
// 			repo = app.RepoName
//
// 			if app.IsRepoCloned() {
// 				isCloned = emoji.Sprint(":heavy_check_mark:")
// 				if app.BranchForRelease {
// 					branch = app.GetBranchName().String()
// 				} else {
// 					branch = ""
// 				}
// 				version = app.Version.String()
// 			} else {
// 				isCloned = emoji.Sprint("    :x:")
// 				branch = ""
// 				version = app.Version.String()
// 			}
//
// 			if app.IsFromManifest {
// 				manifest, _ := app.GetManifest(ctx)
// 				path, _ = filepath.Rel(wd, manifest.AppConfig.FromPath)
// 			} else {
// 				path, _ = filepath.Rel(wd, app.AppConfig.FromPath)
// 			}
// 			t.AddLine(app.Name, isCloned, version, repo, path, branch)
// 		}
//
// 		t.Print()
//
// 		return nil
// 	},
// })

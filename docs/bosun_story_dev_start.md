## bosun story dev start

Start development on a story.

### Synopsis

Start development on a story.

```
bosun story dev start {story} [title] [body] [flags]
```

### Options

```
  -h, --help                 help for start
      --parent-org string    Issue org. (default "naveegoinc")
      --parent-repo string   Issue repo. (default "stories")
```

### Options inherited from parent commands

```
      --cluster string       Set to target a specific cluster.
      --config-file string   Config file for Bosun. You can also set BOSUN_CONFIG. (default "/home/steve/.bosun/bosun.yaml")
      --confirm-env string   Set to confirm that the environment is correct when targeting a protected environment.
      --dry-run              Display rendered plans, but do not actually execute (not supported by all commands).
      --force                Force the requested command to be executed even if heuristics indicate it should not be.
      --no-report            Disable reporting of deploys to github.
  -o, --output table         Output format. Options are table, `json`, or `yaml`. Only respected by a some commands.
      --sudo                 Use sudo when running commands like docker.
      --verbose              Enable verbose logging.
  -V, --verbose-errors       Enable verbose errors with stack traces.
```

### SEE ALSO

* [bosun story dev](bosun_story_dev.md)	 - Commands related to developing the story.

###### Auto generated by spf13/cobra on 27-Oct-2021
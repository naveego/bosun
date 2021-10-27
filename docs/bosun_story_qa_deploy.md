## bosun story qa deploy

Deploy the apps for a story to the current stack. If the current stack doesn't have a story assigned you will be prompted to set it.

### Synopsis

Deploy the apps for a story to the current stack. If the current stack doesn't have a story assigned you will be prompted to set it.

```
bosun story qa deploy [apps...] [flags]
```

### Options

```
      --diff-only         Display the diffs for the deploy, but do not actually execute.
  -h, --help              help for deploy
      --skip-validation   Skip validation of the deployment
      --validate-only     Only validate the deployment
      --values-only       Display the values which would be used for the deploy, but do not actually execute.
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

* [bosun story qa](bosun_story_qa.md)	 - Commands related to testing the story.

###### Auto generated by spf13/cobra on 27-Oct-2021
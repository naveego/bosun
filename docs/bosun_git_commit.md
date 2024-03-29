## bosun git commit

Commits with a formatted message

### Synopsis

Commits with a formatted message

```
bosun git commit [flags]
```

### Options

```
  -h, --help                    help for commit
  -r, --retry                   Commits with the previously failed commit message.
      --squash-against string   Squash all commits since branching off the provided branch.
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

* [bosun git](bosun_git.md)	 - Git commands.

###### Auto generated by spf13/cobra on 27-Oct-2021

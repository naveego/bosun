## bosun stack configure pull-secrets

Configures pull-secrets for the provided stack. Uses the current stack if none is provided.

### Synopsis

Configures pull-secrets for the provided stack. Uses the current stack if none is provided.

```
bosun stack configure pull-secrets [name] [flags]
```

### Options

```
  -h, --help   help for pull-secrets
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

* [bosun stack configure](bosun_stack_configure.md)	 - Configures namespaces and other things for the provided stack. Uses the current stack if none is provided.

###### Auto generated by spf13/cobra on 27-Oct-2021

## bosun app recycle

Recycles the requested app(s) by deleting their pods.

### Synopsis

If app is not specified, the first app in the nearest bosun.yaml file is used.

```
bosun app recycle [name] [name...] [flags]
```

### Options

```
  -h, --help          help for recycle
      --pull-latest   Pull the latest image before recycling (only works in minikube).
```

### Options inherited from parent commands

```
  -a, --all                  ApplyToValues to all known microservices.
      --cluster string       Set to target a specific cluster.
      --config-file string   Config file for Bosun. You can also set BOSUN_CONFIG. (default "/home/steve/.bosun/bosun.yaml")
      --confirm-env string   Set to confirm that the environment is correct when targeting a protected environment.
      --dry-run              Display rendered plans, but do not actually execute (not supported by all commands).
      --exclude strings      Don't include apps which match the provided selectors.".
      --force                Force the requested command to be executed even if heuristics indicate it should not be.
      --include strings      Only include apps which match the provided selectors. --include trumps --exclude.".
  -i, --labels strings       ApplyToValues to microservices with the provided labels.
      --no-report            Disable reporting of deploys to github.
  -o, --output table         Output format. Options are table, `json`, or `yaml`. Only respected by a some commands.
  -p, --providers strings    The priority of the app providers used to get the apps. (default [workspace,unstable,stable,file])
      --sudo                 Use sudo when running commands like docker.
      --verbose              Enable verbose logging.
  -V, --verbose-errors       Enable verbose errors with stack traces.
```

### SEE ALSO

* [bosun app](bosun_app.md)	 - App commands

###### Auto generated by spf13/cobra on 27-Oct-2021

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
  -a, --all                  Apply to all known microservices.
      --config-file string   Config file for Bosun. You can also set BOSUN_CONFIG. (default "$HOME/.bosun/bosun.yaml")
      --dry-run              Display rendered plans, but do not actually execute (not supported by all commands).
      --exclude strings      Don't include apps which match the provided selectors.".
      --force                Force the requested command to be executed even if heuristics indicate it should not be.
      --include strings      Only include apps which match the provided selectors. --include trumps --exclude.".
  -i, --labels strings       Apply to microservices with the provided labels.
      --no-report            Disable reporting of deploys to github.
  -o, --output table         Output format. Options are table, `json`, or `yaml`. Only respected by a some commands. (default "yaml")
      --verbose              Enable verbose logging.
```

### SEE ALSO

* [bosun app](bosun_app.md)	 - App commands

###### Auto generated by spf13/cobra on 16-May-2019

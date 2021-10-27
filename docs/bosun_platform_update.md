## bosun platform update

Updates the manifests of the provided apps on the unstable branch with the provided apps. Defaults to using the 'develop' branch of the apps.

### Synopsis

Updates the manifests of the provided apps on the unstable branch with the provided apps. Defaults to using the 'develop' branch of the apps.

```
bosun platform update {stable|unstable} [names...] [flags]
```

### Options

```
      --all               Will include all items.
      --branch string     The branch to update from.
      --deployed          If set, updates all apps currently marked to be deployed in the release.
      --exclude strings   Will exclude items with labels matching filter (like x==y or x?=prefix-.*).
  -h, --help              help for update
      --include strings   Will include items with labels matching filter (like x==y or x?=prefix-.*).
      --known             If set, updates all apps currently in the release.
      --labels strings    Will include any items where a label with that key is present.
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

* [bosun platform](bosun_platform.md)	 - Contains platform related sub-commands.

###### Auto generated by spf13/cobra on 27-Oct-2021
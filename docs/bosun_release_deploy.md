## bosun release deploy

Deploys the release.

### Synopsis

Deploys the current release to the current environment.

```
bosun release deploy [flags]
```

### Options

```
      --all                  Will include all items.
      --exclude strings      Will exclude items with labels matching filter (like x==y or x?=prefix-.*).
  -h, --help                 help for deploy
      --include strings      Will include items with labels matching filter (like x==y or x?=prefix-.*).
      --labels strings       Will include any items where a label with that key is present.
  -s, --set strings          Value overrides to set in this deploy, as path.to.key=value pairs.
      --skip-validation      Skips running validation before deploying the release.
  -v, --value-sets strings   Additional value sets to include in this deploy.
```

### Options inherited from parent commands

```
      --config-file string           Config file for Bosun. You can also set BOSUN_CONFIG. (default "$HOME/.bosun/bosun.yaml")
      --dry-run                      Display rendered plans, but do not actually execute (not supported by all commands).
      --force                        Force the requested command to be executed even if heuristics indicate it should not be.
      --no-report                    Disable reporting of deploys to github.
  -o, --output table                 Output format. Options are table, `json`, or `yaml`. Only respected by a some commands. (default "yaml")
  -r, --release release use {name}   The release to use for this command (overrides current release set with release use {name}).
      --verbose                      Enable verbose logging.
```

### SEE ALSO

* [bosun release](bosun_release.md)	 - Contains sub-commands for releases.

###### Auto generated by spf13/cobra on 16-May-2019

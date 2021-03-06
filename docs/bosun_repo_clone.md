## bosun repo clone

Clones the named repo(s).

### Synopsis

Uses the first directory in `gitRoots` from the root config.

```
bosun repo clone {name} [name...] [flags]
```

### Options

```
      --all               Will include all items.
      --dir org/repo      The directory to clone into. (The repo will be cloned into org/repo in this directory.) 
      --exclude strings   Will exclude items with labels matching filter (like x==y or x?=prefix-.*).
  -h, --help              help for clone
      --include strings   Will include items with labels matching filter (like x==y or x?=prefix-.*).
      --labels strings    Will include any items where a label with that key is present.
```

### Options inherited from parent commands

```
      --config-file string   Config file for Bosun. You can also set BOSUN_CONFIG. (default "$HOME/.bosun/bosun.yaml")
      --dry-run              Display rendered plans, but do not actually execute (not supported by all commands).
      --force                Force the requested command to be executed even if heuristics indicate it should not be.
      --no-report            Disable reporting of deploys to github.
  -o, --output table         Output format. Options are table, `json`, or `yaml`. Only respected by a some commands. (default "yaml")
      --verbose              Enable verbose logging.
```

### SEE ALSO

* [bosun repo](bosun_repo.md)	 - Contains sub-commands for interacting with repos. Has some overlap with the git sub-command.

###### Auto generated by spf13/cobra on 16-May-2019

## bosun release deploy

Deploys the release.

### Synopsis

Deploys the current release to the current environment.

```
bosun release deploy [flags]
```

### Options

```
  -h, --help   help for deploy
```

### Options inherited from parent commands

```
      --ci-mode              Operate in CI mode, reporting deployments and builds to github.
      --config-file string   Config file for Bosun. (default "$HOME/.bosun/bosun.yaml")
      --dry-run              Display rendered plans, but do not actually execute (not supported by all commands).
      --force                Force the requested command to be executed even if heuristics indicate it should not be.
      --verbose              Enable verbose logging.
```

### SEE ALSO

* [bosun release](bosun_release.md)	 - Release commands.

###### Auto generated by spf13/cobra on 27-Dec-2018
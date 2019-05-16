## bosun meta

Commands for managing bosun itself.

### Synopsis

Commands for managing bosun itself.

### Options

```
  -h, --help   help for meta
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

* [bosun](bosun.md)	 - Devops tool.
* [bosun meta downgrade](bosun_meta_downgrade.md)	 - Downgrades bosun to a previous release.
* [bosun meta upgrade](bosun_meta_upgrade.md)	 - Upgrades bosun if a newer release is available
* [bosun meta version](bosun_meta_version.md)	 - Shows bosun version

###### Auto generated by spf13/cobra on 16-May-2019
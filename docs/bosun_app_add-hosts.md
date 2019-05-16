## bosun app add-hosts

Writes out what the hosts file apps to the hosts file would look like if the requested apps were bound to the minikube IP.

### Synopsis

Writes out what the hosts file apps to the hosts file would look like if the requested apps were bound to the minikube IP.

The current domain and the minikube IP are used to populate the output. To update the hosts file, pipe to sudo tee /etc/hosts.

```
bosun app add-hosts [name...] [flags]
```

### Examples

```
bosun apps add-hosts --all | sudo tee /etc/hosts
```

### Options

```
  -h, --help   help for add-hosts
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
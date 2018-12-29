## bosun vault

Updates VaultClient using layout files. Supports --dry-run flag.

### Synopsis

This command has environmental pre-reqs:
- You must be authenticated to vault (with VAULT_ADDR set and either VAULT_TOKEN set or a ~/.vault-token file created by logging in to vault).

The {vault-layouts...} argument is one or more paths to a vault layout yaml, or a glob which will locate a set of files.

The vault layout yaml file can use go template syntax for formatting.

The .Domain and .Cluster values are populated from the flags to this command, or inferred from VAULT_ADDR.
Any values provided using --values will be in {{ .Values.xxx }}


```
bosun vault {vault-layouts...} [flags]
```

### Examples

```
vault green-auth.yaml green-kube.yaml green-default.yaml
```

### Options

```
  -h, --help                 help for vault
      --vault-addr string    URL to Vault. Or set VAULT_ADDR.
      --vault-token string   Vault token. Or set VAULT_TOKEN.
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

* [bosun](bosun.md)	 - Devops tool.
* [bosun vault bootstrap-dev](bosun_vault_bootstrap-dev.md)	 - Sets up a Vault instance suitable for non-production environment.
* [bosun vault jwt](bosun_vault_jwt.md)	 - Creates a JWT.
* [bosun vault secret](bosun_vault_secret.md)	 - Gets a secret value from vault, optionally populating the value if not found.
* [bosun vault unseal](bosun_vault_unseal.md)	 - Unseals vault using the keys at the provided path, if it exists. Intended to be run from within kubernetes, with the shard secret mounted.

###### Auto generated by spf13/cobra on 27-Dec-2018
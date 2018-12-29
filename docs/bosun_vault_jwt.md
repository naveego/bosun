## bosun vault jwt

Creates a JWT.

### Synopsis

Creates a JWT.

```
bosun vault jwt [flags]
```

### Examples

```
vault init-dev
```

### Options

```
      --claims strings       Additional claims to set, as k=v pairs. Use the flag multiple times or delimit claims with commas.
  -h, --help                 help for jwt
  -r, --role string          The role to use when creating the token. (default "auth")
  -s, --sub string           The sub to set. (default "steve")
  -t, --tenant string        The tenant to set.
      --ttl duration         The TTL for the JWT, in go duration format. (default 15m0s)
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

* [bosun vault](bosun_vault.md)	 - Updates VaultClient using layout files. Supports --dry-run flag.

###### Auto generated by spf13/cobra on 27-Dec-2018
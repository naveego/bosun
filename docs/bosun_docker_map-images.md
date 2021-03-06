## bosun docker map-images

Retags a list of images

### Synopsis

Provide a file with images mapped like
	
x/imageA:0.2.1 y/imageA:0.2.1
x/imageB:0.5.0 x/imageB:0.5.0-rc


```
bosun docker map-images {map file} [flags]
```

### Options

```
  -h, --help   help for map-images
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

* [bosun docker](bosun_docker.md)	 - Group of docker-related commands.

###### Auto generated by spf13/cobra on 16-May-2019

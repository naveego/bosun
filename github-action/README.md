# setup-bosun

This action installs the [bosun](https://github.com/naveego/bosun) executable.
 
## Usage

Basic

```
steps:
- uses: actions/checkout@master
- uses: actions/setup-bosun@master
- run: bosun --version
```

## Development

After making any changes in the ./src files you must build and commit the action. Run this command in this directory:

```
npm run build
```


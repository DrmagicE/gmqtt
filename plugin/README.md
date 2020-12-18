# Plugin

## How to write plugins
Gmqtt uses code generator to generate plugin template. 

First, install the CLI tool:
```bash
# run under gmqtt project root directory. 
go install ./cmd/gmqctl 
```
Enjoy: 
```bash
$ gmqctl gen plugin --help
code generator

Usage:
  gmqctl gen plugin [flags]

Examples:
The following command will generate a code template for the 'awesome' plugin, which makes use of OnConnect and OnSubscribe hook and enables the configuration in ./plugins directory.

gmqctl gen plugin -n awesome -H OnConnect,OnSubscribe -c true -o ./plugins

Flags:
  -c, --config          Whether the plugin needs a configuration.
  -h, --help            help for plugin
  -H, --hooks string    The hooks use by the plugin, multiple hooks are separated by ','
  -n, --name string     The plugin name.
  -o, --output string   The output directory.

```

Details...TODO

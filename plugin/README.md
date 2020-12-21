# Plugin
[Gmqtt插件机制详解](https://juejin.cn/post/6908305981923409934)
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
The following command will generate a code template for the 'awesome' plugin, which makes use of OnBasicAuth and OnSubscribe hook and enables the configuration in ./plugin directory.

gmqctl gen plugin -n awesome -H OnBasicAuth,OnSubscribe -c true -o ./plugin

Flags:
  -c, --config          Whether the plugin needs a configuration.
  -h, --help            help for plugin
  -H, --hooks string    The hooks use by the plugin, multiple hooks are separated by ','
  -n, --name string     The plugin name.
  -o, --output string   The output directory.

```

Details...TODO

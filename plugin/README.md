# Plugin
[Gmqtt插件机制详解](https://juejin.cn/post/6908305981923409934)
## How to write plugins
Gmqtt uses code generator to generate plugin template.

## 1. Install the CLI tool
```bash
# run under gmqtt project root directory. 
go install ./cmd/gmqctl 
```
## 2. Run `gmqctl gen plugin`
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

## 3. Edit `plugin_imports.yml`
Append your plugin name to `plugin_imports.yml`.
```yaml
packages:
  - admin
  - prometheus
  - federation
  - auth
  # for external plugin, use full import path
  # - github.com/DrmagicE/gmqtt/plugin/prometheu
```

## 4. Run `go generate ./...`
Run `go generate ./...` under the project root directory. The command will recreate the `./cmd/gmqttd/plugins.go` file, 
which is needed during the compile time.
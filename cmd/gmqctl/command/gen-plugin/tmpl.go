package gen_plugin

import (
	"bytes"
	"fmt"
	"go/format"
	"reflect"
	"strings"
	"text/template"

	"github.com/DrmagicE/gmqtt/server"
)

const MainTemplate = `package {{.Name}}

import (
	"go.uber.org/zap"

	"github.com/DrmagicE/gmqtt/config"
	"github.com/DrmagicE/gmqtt/server"
)

var _ server.Plugin = (*{{.StrutName}})(nil)

const Name = "{{.Name}}"

func init() {
	server.RegisterPlugin(Name, New)
	{{- if .Config}}
	config.RegisterDefaultPluginConfig(Name, &DefaultConfig)
	{{- end}}
}

func New(config config.Config) (server.Plugin, error) {
    panic("implement me")
}

var log *zap.Logger

type {{.StrutName}} struct {
}

func ({{.Receiver}} *{{.StrutName}}) Load(service server.Server) error {
	log = server.LoggerWithField(zap.String("plugin",Name))
	panic("implement me")
}

func ({{.Receiver}} *{{.StrutName}}) Unload() error {
	panic("implement me")
}

func ({{.Receiver}} *{{.StrutName}}) Name() string {
	return Name
}`

const HookTemplate = `package {{.Name}}

import (
	"github.com/DrmagicE/gmqtt/server"
)

func ({{.Receiver}} *{{.StrutName}}) HookWrapper() server.HookWrapper {
	{{- if .Hooks}}
	return server.HookWrapper{
	    {{- range $index, $element := .Hooks}}
	    {{$element}}Wrapper: {{ $.Receiver}}.{{$element}}Wrapper,
	    {{- end}}
	}
	{{- else}}
	return server.HookWrapper{}
	{{- end}}
}

{{range $index, $element := .Hooks}}
func ({{$.Receiver}} *{{$.StrutName}}) {{$element}}Wrapper(pre server.{{$element}}) server.{{$element}} {
	panic("impermanent me")
}
{{end}}
`

const ConfigTemplate = `package {{.Name}}

{{if .Config}}
// Config is the configuration for the {{.Name}} plugin.
type Config struct {
	// add your config fields
}

// Validate validates the configuration, and return an error if it is invalid.
func (c *Config) Validate() error {
	panic("implement me")
}

// DefaultConfig is the default configuration.
var DefaultConfig = Config{

}

func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
    panic("implement me")
}
{{- end}}
`

// Var is the template variable for all templates
type Var struct {
	// Name is the plugin name. Snack style.
	Name string
	// StrutName is the struct name. Camel style.
	StrutName string
	// Receiver is the struct method receiver.
	Receiver string
	// Hooks is the hooks used in the plugin.
	Hooks []string
	// Config indicates whether the plugin use  configuration.
	Config bool
}

// Execute executes the template into file with Var as variable.
func (v *Var) Execute(file, tmplName, tmpl string) error {
	t, err := template.New(tmplName).Parse(tmpl)
	if err != nil {
		return err
	}
	f, err := openFile(file)
	if err != nil {
		return err
	}
	buf := &bytes.Buffer{}
	err = t.Execute(buf, v)
	if err != nil {
		return err
	}
	out, err := format.Source(buf.Bytes())
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(out)
	return err
}

// ValidateHookName returns an error when the given hook name is invalid
func ValidateHookName(name string) error {
	h := server.Hooks{}
	rfl := reflect.TypeOf(h)
	for i := 0; i < rfl.NumField(); i++ {
		if name == rfl.Field(i).Name {
			return nil
		}
	}
	return fmt.Errorf("invalid hook name: %s", name)
}

// ValidateHooks parse the input string into hook name slice.
// Return an error when any of the names is invalid.
func ValidateHooks(str string) (names []string, err error) {
	if str == "" {
		return []string{}, nil
	}
	names = strings.Split(str, ",")
	for k, v := range names {
		names[k] = strings.TrimSpace(v)
		err := ValidateHookName(names[k])
		if err != nil {
			return nil, err
		}
	}
	return names, nil
}

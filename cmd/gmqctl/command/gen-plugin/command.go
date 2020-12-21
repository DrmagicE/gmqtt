package gen_plugin

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/iancoleman/strcase"
	"github.com/spf13/cobra"
)

var (
	name     string
	hooksStr string
	config   bool
	output   string
)

func must(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	Command.Run = func(cmd *cobra.Command, args []string) {
		must(run(cmd, args))
	}
	Command.Flags().StringVarP(&name, "name", "n", "", "The plugin name.")
	Command.Flags().StringVarP(&hooksStr, "hooks", "H", "", "The hooks use by the plugin, multiple hooks are separated by ','")
	Command.Flags().BoolVarP(&config, "config", "c", false, "Whether the plugin needs a configuration.")
	Command.Flags().StringVarP(&output, "output", "o", "", "The output directory.")
	Command.MarkFlagRequired("name")
}

func prepareOutput(output string) error {
	fi, err := os.Stat(output)
	if err != nil {
		if os.IsExist(err) && !fi.IsDir() {
			return fmt.Errorf("output directory cannot be a file: %s", output)
		}
		if os.IsNotExist(err) {
			err = os.MkdirAll(output, 0777)
		}
	}
	return err
}

func openFile(name string) (f *os.File, err error) {
	_, err = os.Stat(name)
	if os.IsNotExist(err) {
		return os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	}
	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("file %s already exists", name)
}

var Command = &cobra.Command{
	Use:   "plugin",
	Short: "code generator",
	Example: "The following command will generate a code template for the 'awesome' plugin, which makes use of OnBasicAuth and OnSubscribe hook and enables the configuration in ./plugin directory.\n" +
		"\n" +
		"gmqctl gen plugin -n awesome -H OnBasicAuth,OnSubscribe -c true -o ./plugin",
}

func run(cmd *cobra.Command, args []string) error {
	name := strings.ToLower(name)
	// Guess the output directory when not set.
	// If the current directory is gmqtt, then the code gen assumes it is in the gmqtt project root directory.
	if output == "" {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		_, f := path.Split(dir)
		if f == "gmqtt" {
			output = path.Join(dir, "plugin", name)
		}
	}
	if output == "" {
		return errors.New("missing output")
	}

	hooks, err := ValidateHooks(hooksStr)
	if err != nil {
		return err
	}
	v := &Var{
		Name:      strcase.ToSnake(name),
		StrutName: strcase.ToCamel(name),
		Hooks:     hooks,
		Receiver:  string(name[0]),
		Config:    config,
	}

	err = prepareOutput(output)
	if err != nil {
		return err
	}

	err = v.Execute(path.Join(output, name+".go"), "main", MainTemplate)
	if err != nil {
		return err
	}

	err = v.Execute(path.Join(output, "hooks.go"), "hook", HookTemplate)
	if err != nil {
		return err
	}

	if v.Config {
		err = v.Execute(path.Join(output, "config.go"), "config", ConfigTemplate)
	}
	return err
}

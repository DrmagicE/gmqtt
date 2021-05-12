// +build pprof

package main

import (
	_ "net/http/pprof"
)

func init() {
	enablePprof = true
	rootCmd.PersistentFlags().StringVar(&pprofAddr, "pprof_addr", pprofAddr, "The listening address for the pprof http server")
}

// Command configconv converts the original MuServer configuration (.ini,
// .dat and Data/**.txt files) into the TOML files the server reads under
// apps/server/config.
//
//	configconv -from <MuServer folder> [-to config] [-only server,gameserver,item,...]
//
// The source folder must have the original layout: 1.ConnectServer,
// 3.JoinServer, 4.GameServer/DATA, 5.GameServerCS/DATA and Data.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/legacy"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "configconv:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("configconv", flag.ContinueOnError)
	from := fs.String("from", "", "original MuServer folder (1.ConnectServer, 4.GameServer, Data, ...)")
	to := fs.String("to", "config", "config directory to write")
	only := fs.String("only", "", "comma-separated groups to convert (default all)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *from == "" {
		return fmt.Errorf("-from is required")
	}
	groups := map[string]bool{}
	known := map[string]bool{}
	for _, o := range legacy.Outputs() {
		known[o.Group] = true
	}
	for _, g := range strings.Split(*only, ",") {
		if g = strings.TrimSpace(g); g == "" {
			continue
		}
		if !known[g] {
			return fmt.Errorf("unknown group %q", g)
		}
		groups[g] = true
	}
	written, err := legacy.Convert(*from, *to, groups)
	for _, p := range written {
		fmt.Fprintln(stdout, "wrote", p)
	}
	return err
}

// Command migrate copies an original MuOnline database into the server's
// PostgreSQL schema.
//
//	migrate -from sqlserver://sa:pass@host:1433?database=MuOnline -to postgres://...
//	migrate -script data/DB/MuOnline.sql -to postgres://...
//	migrate -gamemasters <MuServer folder> -to postgres://...
//
// The target must not have accounts yet; everything runs in one
// transaction. -gamemasters copies Data/Util/GameMaster.txt into
// characters.gm_level, alone or after the database import.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/jackc/pgx/v5"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/legacy"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/migrate"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	from := fs.String("from", "", "original SQL Server URL (sqlserver://...)")
	script := fs.String("script", "", "SQL Server script with INSERT statements (instead of -from)")
	gmRoot := fs.String("gamemasters", "", "MuServer folder whose Data/Util/GameMaster.txt sets characters.gm_level")
	to := fs.String("to", os.Getenv("MU_DATABASE_URL"), "target PostgreSQL URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *to == "" || (*from != "" && *script != "") || (*from == "" && *script == "" && *gmRoot == "") {
		return errors.New("use -to and -from, -script or -gamemasters (-from and -script exclude each other)")
	}
	var gms []legacy.GameMaster
	if *gmRoot != "" {
		var err error
		if gms, err = legacy.ReadGameMasters(*gmRoot); err != nil {
			return err
		}
	}
	var src migrate.Source
	if *script != "" {
		s, err := migrate.LoadScript(*script)
		if err != nil {
			return err
		}
		src = s
	} else if *from != "" {
		s, err := migrate.OpenSQLServer(ctx, *from)
		if err != nil {
			return err
		}
		defer s.Close()
		src = s
	}
	// OpenPostgres creates or updates the schema.
	pg, err := store.OpenPostgres(ctx, *to)
	if err != nil {
		return err
	}
	pg.Close()
	conn, err := pgx.Connect(ctx, *to)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var rep *migrate.Report
	if src != nil {
		if rep, err = migrate.Run(ctx, src, tx); err != nil {
			return err
		}
	}
	gmSet, gmMissing, err := migrate.ImportGameMasters(ctx, tx, gms)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if rep != nil {
		fmt.Fprintf(out, "accounts %d, characters %d, warehouses %d, options %d, friends %d (%d links, %d mails), guilds %d (%d members), pets %d, event entries %d, golden archer %d\n",
			rep.Accounts, rep.Characters, rep.Warehouses, rep.Options, rep.Friends, rep.FriendLinks, rep.Mails,
			rep.Guilds, rep.GuildMembers, rep.Pets, rep.EventEntries, rep.Renas)
		for _, s := range rep.Skipped {
			fmt.Fprintln(out, "skipped:", s)
		}
	}
	if *gmRoot != "" {
		fmt.Fprintf(out, "game masters %d\n", gmSet)
		for _, s := range gmMissing {
			fmt.Fprintln(out, "game master not found:", s)
		}
	}
	return nil
}

package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/mmulyana/my-todo-runner/internal/config"
	"github.com/mmulyana/my-todo-runner/internal/tick"
)

const usage = `usage: my-todo-runner run-once [--config path] [--dry-run]`

func main() {
	if len(os.Args) < 2 || os.Args[1] != "run-once" {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}

	flags := flag.NewFlagSet("run-once", flag.ExitOnError)
	configPath := flags.String("config", "/etc/my-todo-runner/config.json", "path config.json")
	dryRun := flags.Bool("dry-run", false, "tampilkan kartu yang akan diambil tanpa mengubah apa pun")
	_ = flags.Parse(os.Args[2:])

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Error("config tidak valid", "err", err)
		os.Exit(1)
	}

	// note: systemd mengirim SIGTERM saat stop/timeout, context dibatalkan biar opencode ikut dihentikan
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := tick.Run(ctx, cfg, *dryRun, log); err != nil {
		log.Error("tick gagal", "err", err)
		os.Exit(1)
	}
}

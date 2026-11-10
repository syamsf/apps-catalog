package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/rs/zerolog"

	"apps-catalog/internal/api"
	"apps-catalog/internal/auth"
	"apps-catalog/internal/catalog"
	"apps-catalog/internal/config"
	"apps-catalog/internal/health"
	"apps-catalog/internal/store"
	"apps-catalog/internal/web"
)

var version = "dev"

const usage = `usage:
  portal serve [-config file]           start the API and web UI
  portal import [-config file] [-update] [file|dir]
                                        add apps from an import file (docs/import.md)
                                        or a YAML catalog dir; apps that already
                                        exist are skipped, -update replaces them
                                        (files only, dir defaults to catalog.dir)
  portal user add [-config file] [-admin] [-name n] [-email e] <username>
                                        add a local user, asks for the password
  portal user passwd [-config file] <username>
                                        set a new password for a local user
  portal validate <dir>                 check the YAML catalog, exits 1 on errors
  portal version

Settings come from the optional config file (see config.example.yaml)
and from env vars, which win. A key like health.interval is read from
HEALTH_INTERVAL.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch cmd, args := os.Args[1], os.Args[2:]; cmd {
	case "serve":
		err = serve(ctx, args)
	case "import":
		err = importDir(ctx, args)
	case "user":
		err = userCmd(ctx, args)
	case "validate":
		if len(args) != 1 {
			fmt.Fprint(os.Stderr, usage)
			os.Exit(2)
		}
		err = validate(args[0])
	case "version":
		fmt.Println(version)
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// loadConfig parses -config from args and returns the config and the
// remaining args.
func loadConfig(name string, args []string, extra ...func(*flag.FlagSet)) (config.Config, []string, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	file := fs.String("config", os.Getenv("CONFIG_FILE"), "yaml config file")
	for _, f := range extra {
		f(fs)
	}
	if err := fs.Parse(args); err != nil {
		return config.Config{}, nil, err
	}
	cfg, err := config.Load(*file)
	return cfg, fs.Args(), err
}

func newLogger(c config.Log) zerolog.Logger {
	level, err := zerolog.ParseLevel(strings.ToLower(c.Level))
	if err != nil || level == zerolog.NoLevel {
		level = zerolog.InfoLevel
	}
	zerolog.TimeFieldFormat = time.RFC3339Nano
	if c.Format == "console" {
		return zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.TimeOnly}).
			Level(level).With().Timestamp().Logger()
	}
	return zerolog.New(os.Stderr).Level(level).With().Timestamp().Logger()
}

func validate(dir string) error {
	apps, err := catalog.LoadDir(dir)
	if err != nil {
		return err
	}
	fmt.Printf("ok: %d apps\n", len(apps))
	return nil
}

func importDir(ctx context.Context, args []string) error {
	var update bool
	cfg, rest, err := loadConfig("import", args, func(fs *flag.FlagSet) {
		fs.BoolVar(&update, "update", false, "replace apps that already exist (import files only)")
	})
	if err != nil {
		return err
	}
	dir := cfg.Catalog.Dir
	if len(rest) > 0 {
		dir = rest[0]
	}
	if dir == "" {
		return errors.New("no catalog dir: pass one or set catalog.dir (CATALOG_DIR)")
	}
	if cfg.Database.URL == "" {
		return errors.New("database.url (DATABASE_URL) is required")
	}
	if info, err := os.Stat(dir); err != nil {
		return err
	} else if !info.IsDir() {
		return importFile(ctx, cfg.Database.URL, dir, update)
	}
	if update {
		return errors.New("-update only works with an import file, not a catalog dir")
	}
	apps, err := catalog.LoadDir(dir)
	if err != nil {
		return fmt.Errorf("catalog %s is invalid, nothing imported: %w", dir, err)
	}
	st, err := store.Open(ctx, cfg.Database.URL)
	if err != nil {
		return err
	}
	defer st.Close()
	res, err := st.Import(ctx, store.Actor{Subject: "import", Name: "Import"}, apps)
	if err != nil {
		return err
	}
	fmt.Printf("added %d apps", len(res.Added))
	if len(res.Skipped) > 0 {
		fmt.Printf(", skipped %d that already exist: %s", len(res.Skipped), strings.Join(res.Skipped, ", "))
	}
	fmt.Println()
	return nil
}

// importFile imports one file in the version 1 format, see docs/import.md.
func importFile(ctx context.Context, dbURL, path string, update bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	st, err := store.Open(ctx, dbURL)
	if err != nil {
		return err
	}
	defer st.Close()
	res, plan, err := st.ApplyImport(ctx, store.Actor{Subject: "import", Name: "Import"}, data, update)
	if errors.Is(err, store.ErrImportInvalid) {
		for _, p := range plan.Problems {
			where := fmt.Sprintf("%s:%d", path, p.Line)
			if p.App != "" {
				where += " " + p.App
			}
			fmt.Fprintf(os.Stderr, "%s: %s\n", where, p.Message)
		}
	}
	if err != nil {
		return err
	}
	fmt.Printf("added %d, updated %d, skipped %d", len(res.Added), len(res.Updated), len(res.Skipped))
	if len(res.Skipped) > 0 {
		fmt.Printf(" (already exist, use -update to replace them): %s", strings.Join(res.Skipped, ", "))
	}
	fmt.Println()
	for _, a := range plan.Apps {
		if a.OwnerNote != "" {
			fmt.Printf("%s: %s\n", a.ID, a.OwnerNote)
		}
	}
	return nil
}

func serve(ctx context.Context, args []string) error {
	cfg, _, err := loadConfig("serve", args)
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	log := newLogger(cfg.Log)

	st, err := store.Open(ctx, cfg.Database.URL)
	if err != nil {
		return err
	}
	defer st.Close()

	// also used for "test now" on the app form, so built even when checks are off
	checker, err := health.NewChecker(cfg.Health.Timeout, cfg.Health.CAFile)
	if err != nil {
		return err
	}
	if cfg.Health.Interval > 0 {
		go checker.Run(ctx, st, log, cfg.Health.Interval)
	}

	authn, err := setupAuth(ctx, cfg, log, st)
	if err != nil {
		return err
	}
	go cleanupLoop(ctx, log, st)

	app := api.New(st, authn, web.Dist(), log, cfg.Auth.AdminGroup, api.WithProber(checker))
	errc := make(chan error, 1)
	go func() {
		log.Info().Str("addr", cfg.HTTP.Addr).Str("version", version).Msg("listening")
		errc <- app.Listen(cfg.HTTP.Addr, fiber.ListenConfig{DisableStartupMessage: true})
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info().Msg("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return app.ShutdownWithContext(shutdownCtx)
}

func setupAuth(ctx context.Context, cfg config.Config, log zerolog.Logger, st *store.Store) (api.Auth, error) {
	if !cfg.LoginEnabled() {
		log.Warn().Msg("oidc.issuer not set and auth.local_login off, login is off and everyone is an admin")
		return auth.Disabled{}, nil
	}
	if cfg.OIDC.Issuer != "" && cfg.Auth.AdminGroup == "" {
		log.Warn().Msg("auth.admin_group not set, no sso user can open the admin area")
	}
	if cfg.Auth.LocalLogin {
		if n, err := st.LocalAdmins(ctx); err == nil && n == 0 && cfg.OIDC.Issuer == "" {
			log.Warn().Msg("local login is on but there is no local admin yet, add one with: portal user add -admin <username>")
		}
	}
	return auth.New(ctx, auth.Config{
		Issuer:       cfg.OIDC.Issuer,
		ClientID:     cfg.OIDC.ClientID,
		ClientSecret: cfg.OIDC.ClientSecret,
		PublicURL:    cfg.HTTP.PublicURL,
		AdminGroup:   cfg.Auth.AdminGroup,
		SessionTTL:   cfg.Auth.SessionTTL,
		LogoutURL:    cfg.Auth.LogoutURL,
		LocalLogin:   cfg.Auth.LocalLogin,
	}, st, log)
}

func cleanupLoop(ctx context.Context, log zerolog.Logger, st *store.Store) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := st.DeleteExpired(ctx); err != nil {
				log.Error().Err(err).Msg("delete expired sessions")
			}
			if err := st.DeleteOldUsage(ctx); err != nil {
				log.Error().Err(err).Msg("delete old usage")
			}
		}
	}
}

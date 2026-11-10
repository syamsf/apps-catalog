package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"apps-catalog/internal/auth"
	"apps-catalog/internal/catalog"
	"apps-catalog/internal/config"
	"apps-catalog/internal/store"
)

var cli = store.Actor{Subject: "cli", Name: "CLI"}

func userCmd(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("use: portal user add <username> or portal user passwd <username>")
	}
	switch args[0] {
	case "add":
		return userAdd(ctx, args[1:])
	case "passwd":
		return userPasswd(ctx, args[1:])
	}
	return fmt.Errorf("unknown user command %q, use add or passwd", args[0])
}

func userAdd(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("user add", flag.ContinueOnError)
	file := fs.String("config", os.Getenv("CONFIG_FILE"), "yaml config file")
	admin := fs.Bool("admin", false, "make the user a portal admin")
	name := fs.String("name", "", "display name")
	email := fs.String("email", "", "email")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("use: portal user add [-admin] [-name n] [-email e] <username>")
	}
	username := fs.Arg(0)
	if !catalog.ValidID(strings.ToLower(username)) {
		return errors.New("username: use letters, numbers and single dashes")
	}
	st, err := openStore(ctx, *file)
	if err != nil {
		return err
	}
	defer st.Close()

	hash, err := readPassword()
	if err != nil {
		return err
	}
	u, err := st.CreateLocalUser(ctx, cli, store.LocalUserInput{
		Username: username, Name: *name, Email: *email, IsAdmin: *admin,
	}, hash)
	if errors.Is(err, store.ErrConflict) {
		return fmt.Errorf("a local user %q already exists, use portal user passwd to change the password", username)
	}
	if err != nil {
		return err
	}
	role := "user"
	if u.IsAdmin {
		role = "admin"
	}
	fmt.Printf("added %s as %s\n", u.Username, role)
	return nil
}

func userPasswd(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("user passwd", flag.ContinueOnError)
	file := fs.String("config", os.Getenv("CONFIG_FILE"), "yaml config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("use: portal user passwd <username>")
	}
	st, err := openStore(ctx, *file)
	if err != nil {
		return err
	}
	defer st.Close()
	subject, _, err := st.LocalCredentials(ctx, fs.Arg(0))
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("no local user %q", fs.Arg(0))
	}
	if err != nil {
		return err
	}
	hash, err := readPassword()
	if err != nil {
		return err
	}
	if err := st.SetPassword(ctx, &cli, subject, hash); err != nil {
		return err
	}
	fmt.Println("password changed")
	return nil
}

func openStore(ctx context.Context, file string) (*store.Store, error) {
	cfg, err := config.Load(file)
	if err != nil {
		return nil, err
	}
	if cfg.Database.URL == "" {
		return nil, errors.New("database.url (DATABASE_URL) is required")
	}
	return store.Open(ctx, cfg.Database.URL)
}

// readPassword asks twice on a terminal, or reads one line from stdin.
func readPassword() (string, error) {
	var pw string
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		fmt.Fprint(os.Stderr, "Password: ")
		a, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		fmt.Fprint(os.Stderr, "Again: ")
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		if string(a) != string(b) {
			return "", errors.New("the passwords don't match")
		}
		pw = string(a)
	} else {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", errors.New("no password on stdin")
		}
		pw = strings.TrimRight(line, "\r\n")
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return "", fmt.Errorf("password: %w", err)
	}
	return hash, nil
}

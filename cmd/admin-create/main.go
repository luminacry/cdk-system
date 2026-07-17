package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/term"

	"cdk-system/internal/auth"
	"cdk-system/internal/config"
	"cdk-system/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: admin-create <username>")
		os.Exit(1)
	}
	username := strings.TrimSpace(os.Args[1])
	if username == "" {
		fmt.Fprintln(os.Stderr, "username cannot be empty")
		os.Exit(1)
	}

	var password string
	if envPass := os.Getenv("ADMIN_PASSWORD"); envPass != "" {
		password = strings.TrimSpace(envPass)
	} else {
		fmt.Print("Password: ")
		passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to read password: %v\n", err)
			os.Exit(1)
		}
		fmt.Println()
		password = strings.TrimSpace(string(passwordBytes))
	}
	if len(password) < 8 {
		fmt.Fprintln(os.Stderr, "password must be at least 8 characters")
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	hash, err := auth.HashPassword(password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to hash password: %v\n", err)
		os.Exit(1)
	}

	queries := store.NewQueries(pool)
	if _, err := queries.CreateAdmin(ctx, store.CreateAdminParams{
		Username:     username,
		PasswordHash: hash,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create admin: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("admin created/updated successfully")
}

package auth

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/service"
)

// Command handles user/token subcommands. Passwords come from DORY_PASSWORD
// rather than process arguments. A newly generated token is printed only here.
func Command(ctx context.Context, args []string, svc service.Service) (bool, error) {
	if len(args) == 0 || args[0] != "user" && args[0] != "token" {
		return false, nil
	}
	if len(args) < 2 || args[1] != "create" {
		return true, fmt.Errorf("usage: dory %s create --username NAME", args[0])
	}
	fs := flag.NewFlagSet(args[0]+" create", flag.ContinueOnError)
	username := fs.String("username", "", "user name")
	var name, scopes *string
	var ttl *time.Duration
	if args[0] == "token" {
		name = fs.String("name", "", "token name")
		scopes = fs.String("scopes", "read", "comma-separated read,write,admin permissions")
		ttl = fs.Duration("ttl", 0, "token lifetime (0 means no expiration)")
	}
	if err := fs.Parse(args[2:]); err != nil {
		return true, err
	}
	if strings.TrimSpace(*username) == "" || fs.NArg() != 0 {
		return true, fmt.Errorf("--username is required; unexpected positional arguments are not accepted")
	}
	if args[0] == "user" {
		password := os.Getenv("DORY_PASSWORD")
		if len(password) < 12 || len(password) > 4096 {
			return true, fmt.Errorf("DORY_PASSWORD must contain 12 to 4096 bytes")
		}
		if _, err := svc.FindUser(ctx, *username); err == nil {
			return true, fmt.Errorf("user already exists")
		} else if !errors.Is(err, service.ErrNotFound) {
			return true, err
		}
		u, err := svc.Users().Put(ctx, "cli", service.User{Username: *username, PasswordHash: passwordHash(password)})
		if err == nil {
			_, err = fmt.Fprintln(os.Stdout, u.ID)
		}
		return true, err
	}
	if strings.TrimSpace(*name) == "" || *ttl < 0 {
		return true, fmt.Errorf("--name is required and --ttl must be nonnegative")
	}
	permissions := strings.Split(*scopes, ",")
	for _, p := range permissions {
		if p != "read" && p != "write" && p != "admin" {
			return true, fmt.Errorf("invalid scope %q", p)
		}
	}
	u, err := svc.FindUser(ctx, *username)
	if err != nil {
		return true, err
	}
	value := secret()
	token := service.Token{UserID: u.ID, Name: *name, SecretHash: digest(value), Permissions: permissions}
	if *ttl > 0 {
		expires := time.Now().Add(*ttl)
		token.ExpiresAt = &expires
	}
	_, err = svc.Tokens().Put(ctx, u.ID, token)
	if err == nil {
		_, err = fmt.Fprintln(os.Stdout, value)
	}
	return true, err
}

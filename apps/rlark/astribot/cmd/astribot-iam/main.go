package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rlinf/rlark/apps/rlark/astribot/iam"
	"github.com/rlinf/rlark/apps/rlark/pkg/db"
)

func main() {
	addr := flag.String("addr", ":8081", "HTTP listen address")
	dbConfigPath := flag.String("db-config", "/etc/rlark/db-config.yaml", "PostgreSQL config file")
	upstreamPath := flag.String("upstream-tokens", "/etc/rlark/iam/upstream-tokens.json", "IAM upstream service-account tokens file")
	ttl := flag.Duration("session-ttl", 168*time.Hour, "IAM session lifetime")
	cookieSecure := flag.Bool("cookie-secure", false, "Set Secure on the IAM session cookie")
	flag.Parse()
	if *ttl <= 0 {
		log.Fatal("--session-ttl must be positive")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	database, err := db.OpenFromFileConfig(*dbConfigPath)
	if err != nil {
		log.Fatalf("open IAM database: %v", err)
	}
	defer database.Close()
	store := iam.NewPostgresStore(database.DB)
	if err := store.MigrateAndBootstrap(ctx); err != nil {
		log.Fatalf("initialize IAM database: %v", err)
	}
	if err := store.DeleteExpiredSessions(ctx, time.Now().UTC()); err != nil {
		log.Fatalf("clean expired IAM sessions: %v", err)
	}
	tokens, err := loadUpstreamTokens(*upstreamPath)
	if err != nil {
		log.Fatalf("load IAM upstream tokens: %v", err)
	}
	httpServer := &http.Server{Addr: *addr, Handler: iam.NewServer(store, *ttl, *cookieSecure, tokens).Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := store.DeleteExpiredSessions(ctx, time.Now().UTC()); err != nil {
					log.Printf("clean expired IAM sessions: %v", err)
				}
			}
		}
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()
	log.Printf("astribot-iam listening on %s", *addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve IAM: %v", err)
	}
}

func loadUpstreamTokens(filename string) (map[iam.Role]string, error) {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat token file: %w", err)
	}
	if info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("token file permissions must be 0600 or stricter")
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read token file: %w", err)
	}
	var values map[string]string
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("decode token file: %w", err)
	}
	return map[iam.Role]string{iam.RoleAdmin: values["admin"], iam.RoleOperator: values["operator"], iam.RoleViewer: values["user"]}, nil
}

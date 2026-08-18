package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rlinf/rlark/apps/rlark/pkg/sshd"
)

func main() {
	port := flag.String("port", "22", "SSH listen port")
	shell := flag.String("shell", "", "Shell binary path (default: /bin/bash)")
	flag.Parse()

	srv := &sshd.Server{
		Port:  *port,
		Shell: *shell,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		os.Exit(0)
	}()

	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "sshd: %v\n", err)
		os.Exit(1)
	}
}

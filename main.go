package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
)

var srv *pgmusql

const defaultCfg = "default.toml"

func main() {
	isChild := flag.Bool("child", false, "Run as child. (Do not use this flag. It is needed to restart the service.)")
	cfgFile := flag.String("cfg", defaultCfg, "Configuration file")
	flag.Parse()

	log.SetPrefix(fmt.Sprintf("pgmusql PID(%d):", syscall.Getpid()))

	sgnl := make(chan os.Signal, 1)
	defer close(sgnl)
	signal.Notify(sgnl, syscall.SIGUSR1, syscall.SIGTERM, syscall.SIGINT)

	// create and run server
	var err error
	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()
	if srv, err = pgmusqlNew(ctx, *isChild, *cfgFile); err != nil {
		log.Fatalln(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	defer wg.Wait()
	go func() {
		defer wg.Done()
		if err = srv.start(); err != nil && err != http.ErrServerClosed {
			log.Fatalln(err)
		}
	}()

	// handle signals
	for s := range sgnl {
		switch s {
		// shutdown server gracefuly
		case syscall.SIGINT:
			srv.stop()
			return
		// terminate server
		case syscall.SIGTERM:
			srv.terminate()
			os.Exit(66)
		// start child server
		case syscall.SIGUSR1:
			cmd := exec.Command(os.Args[0], "-child")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.ExtraFiles = []*os.File{srv.socketFile}
			if err := cmd.Start(); err != nil {
				log.Fatalln(err)
			}
		}
	}
}

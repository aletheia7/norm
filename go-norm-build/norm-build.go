package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

var exit int

func main() {
	log.SetFlags(log.Lshortfile)
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())
	wg := new(sync.WaitGroup)
	defer os.Exit(exit)
	defer wg.Wait()
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()
		goos := os.Getenv("GOOS")
		if goos == "darwin" {
			goos = "macosx"
		}
		for _, cmd := range []c{
			{ctx: ctx, dir: "norm", cmd: "./waf", args: []string{"distclean", "--color", "yes"}},
			{ctx: ctx, dir: "norm", cmd: "./waf", args: []string{"configure", "--color", "yes"}},
			{ctx: ctx, dir: "norm", cmd: "./waf", args: []string{"--color", "yes"}},
			{ctx: ctx, dir: "norm/makefiles", cmd: "make", args: []string{"-f", "Makefile." + goos, "clean"}},
			{ctx: ctx, dir: "norm/makefiles", cmd: "make", args: []string{"-f", "Makefile." + goos}},
		} {
			select {
			case <-ctx.Done():
				return
			default:
				if err := do_cmd(cmd); err != nil {
					exit = 1
					return
				}
			}
		}
		if os.Getenv("GOOS") == "linux" {
			fmt.Printf("%v: Ignore waf configure warning: Checking for library netfilter_queue     : not found\n", os.Args[0])
			fmt.Printf("%v: It's not used by protokit\n", os.Args[0])
		}
	}()
	for {
		select {
		case <-sigc:
			fmt.Fprintf(os.Stderr, "\n")
			cancel()
		case <-ctx.Done():
			return
		}
	}
}

type c struct {
	ctx  context.Context
	cmd  string
	args []string
	dir  string
	env  []string
}

func do_cmd(in c) error {
	rp, wp, err := os.Pipe()
	if err != nil {
		log.Println(err)
		return err
	}
	defer rp.Close()
	defer wp.Close()
	cmd := exec.CommandContext(in.ctx, in.cmd, in.args...)
	if in.dir != "" {
		cmd.Dir = in.dir
	}
	if in.env != nil {
		cmd.Env = append(os.Environ(), in.env...)
	}
	cmd.Stderr = wp
	cmd.Stdout = wp
	go func() {
		scanner := bufio.NewScanner(rp)
		for {
			select {
			case <-in.ctx.Done():
			default:
				if scanner.Scan() {
					fmt.Println(scanner.Text())
				} else {
					if scanner.Err() != nil {
						log.Println(err)
						return
					}
				}
			}
		}
	}()
	fmt.Printf("%v: %v\n", os.Args[0], strings.Join(cmd.Args, " "))
	if err := cmd.Start(); err != nil {
		switch {
		case in.ctx.Err() != nil:
			return nil
		default:
			log.Println(cmd.Args, err)
			return err
		}
	}
	if err := cmd.Wait(); err != nil {
		switch {
		case in.ctx.Err() != nil:
			return nil
		default:
			log.Println(cmd.Args, err)
			return err
		}
	}
	return nil
}

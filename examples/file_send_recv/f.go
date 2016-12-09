// Copyright 2016 aletheia7. All rights reserved. Use of this source code is
// governed by a BSD-2-Clause license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/aletheia7/norm"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

var (
	ctx              context.Context
	cancel           context.CancelFunc
	wg               = new(sync.WaitGroup)
	send             = flag.Bool("s", false, "File Object Send")
	recv             = flag.Bool("r", false, "File Object Receive")
	trace            = flag.Bool("trace", false, "enable session message trace")
	debug_instance   = flag.Bool("di", false, "debug norm.Instance, not NormSetDebug()")
	norm_debug_level = flag.Uint("d", 0, "NormSetDebugLevel(): 0-12")
	s_addr           = flag.String("saddr", "127.0.0.1", "sender session address")
	s_port           = flag.Int("sport", 6003, "sender session port")
	s_node_id        = flag.Uint("snodeid", 6, "sender node id")
	r_addr           = flag.String("raddr", "0.0.0.0", "receiver session address")
	r_port           = flag.Int("rport", 6003, "receiver session port")
	r_node_id        = flag.Uint("rnodeid", 7, "receiver node id")
	r_cache_path     = flag.String("rcachepath", ``, "receiver cache path directory")
)

func do_send() {
	defer wg.Done()
	defer cancel()
	i, err := norm.Create_instance(wg)
	if err != nil {
		log.Println(err)
		return
	}
	log.Println("norm version:", i.Get_version())
	old := i.Get_debug_level()
	i.Set_debug_level(*norm_debug_level)
	log.Printf("norm debug level: %v -> %v\n", old, *norm_debug_level)
	i.Set_debug(*debug_instance)
	defer i.Destroy()
	log.Printf("sender session: %v:%v\n", *s_addr, *s_port)
	sess, err := i.Create_session(*s_addr, *s_port, norm.Node_id(*s_node_id))
	if err != nil {
		log.Println(err)
		return
	}
	log.Println("trace:", *trace)
	sess.Set_message_trace(*trace)
	sess.Set_tx_only(true, false)
	sess.Set_events(norm.Event_type_all &^ (norm.Event_type_grtt_updated | norm.Event_type_tx_rate_changed))
	defer sess.Destroy()
	if !sess.Add_acking_node(norm.Node_id((*r_node_id))) {
		log.Println("Add_acking_node", err)
		return
	}
	sess.Set_tx_cache_bounds(100e+6, 1000, 2000)
	sess.Set_congestion_control(true, true)
	if !sess.Start_sender(0, 0, 0, 0, 0, 0) {
		log.Println(err)
		return
	}
	defer sess.Stop_sender()
	ct := uint(1)
	var obj *norm.Object
	for _, fn := range flag.Args() {
		if _, err := sess.File_enqueue(fn, []byte(fn)); err != nil {
			log.Println("error file enqueue:", err)
			return
		}
	}
	ct++
	for {
		select {
		case e := <-sess.Events():
			switch e.Type {
			case norm.Event_type_send_error:
				log.Println(e)
			case norm.Event_type_tx_rate_changed:
				log.Println(e, e.Tx_rate)
			case norm.Event_type_grtt_updated, norm.Event_type_tx_object_sent,
				norm.Event_type_tx_object_purged:
				log.Println(e)
			case norm.Event_type_tx_flush_completed:
				log.Println(e)
				if obj != nil {
					sess.Set_watermark(obj, true)
					obj = nil
				}
			default:
				log.Println("main:", e)
			}
		case <-ctx.Done():
			return
		}
	}
}

func do_recv() {
	defer wg.Done()
	defer cancel()
	i, err := norm.Create_instance(wg)
	if err != nil {
		log.Println(err)
		return
	}
	old := i.Get_debug_level()
	i.Set_debug_level(*norm_debug_level)
	log.Printf("norm debug level: %v -> %v\n", old, *norm_debug_level)
	i.Set_debug(*debug_instance)
	defer i.Destroy()
	cp, err := os.Getwd()
	if err != nil {
		log.Println(err)
		return
	}
	if 0 < len(*r_cache_path) {
		cp = *r_cache_path
	}
	log.Println("setting cache directory:", cp)
	if !i.Set_cache_directory(cp) {
		log.Println("error:", err, "set cache directory", *r_cache_path)
		return
	}
	log.Printf("receiver session: %v:%v\n", *r_addr, *r_port)
	sess, err := i.Create_session(*r_addr, *r_port, norm.Node_id(*r_node_id))
	if err != nil {
		log.Println(err)
	}
	defer sess.Destroy()
	sess.Set_message_trace(*trace)
	sess.Set_events(norm.Event_type_all &^ norm.Event_type_grtt_updated)
	sess.Set_default_unicast_nack(true)
	sess.Set_rx_cache_limit(4096)
	if !sess.Start_receiver(1024 * 1024) {
		log.Println(err)
		return
	}
	defer sess.Stop_receiver()
	var start_time time.Time
	for {
		select {
		case e := <-sess.Events():
			switch e.Type {
			case norm.Event_type_remote_sender_new, norm.Event_type_remote_sender_active, norm.Event_type_remote_sender_inactive:
				log.Println("main:", e, e.Node.Get_id(), e.Node.Get_address_all())
				start_time = time.Now()
			case norm.Event_type_rx_object_info:
				log.Println("main:", e, e.O.Get_info())
			case norm.Event_type_rx_object_updated:
				if pending := e.O.Get_bytes_pending(); pending == 0 {
					size := e.O.Get_size()
					ocompleted := size - pending
					percent := 100.00 * (float32(ocompleted) / float32(size))
					log.Printf("%v: Id: %v, completion status %v/%v %6.2f%%\n", os.Args[0], e.O.Id, ocompleted, size, percent)
					duration := time.Now().Sub(start_time).Seconds()
					log.Printf("id: %v, transfer duration: %.6f sec at %.6f kbps\n",
						e.O.Id,
						time.Now().Sub(start_time).Seconds(),
						8.0/1000.0*float64(size)/duration,
					)
				}
			case norm.Event_type_rx_object_completed:
				new_fn := filepath.Join(filepath.Dir(e.O.File_get_name().String()), filepath.Base(e.O.Get_info().String()))
				if !e.O.File_rename(new_fn) {
					log.Println("error renaming file:", new_fn)
				}
				e.O.Release()
				log.Println("file ready:", new_fn, "size:", e.O.Get_size())
			default:
				log.Println("main:", e, e.O)
			}
		case <-ctx.Done():
			return
		}
	}
}

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds | log.Lshortfile)
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	defer wg.Wait()
	flag.Usage = func() {
		fmt.Printf(`
SYNOPSIS                                                                                                                                                                                                             
	%s [options] <file_name> [, <file_name>]

`, os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	switch {
	case (!*send && !*recv) || (*send && *recv):
		flag.PrintDefaults()
		return
	case *send:
		if len(flag.Args()) == 0 {
			flag.Usage()
			return
		}
		wg.Add(1)
		go do_send()
	case *recv:
		wg.Add(1)
		go do_recv()
	}
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

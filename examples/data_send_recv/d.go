// Copyright 2016 aletheia7. All rights reserved. Use of this source code is
// governed by a BSD-2-Clause license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"github.com/aletheia7/norm"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var (
	ctx              context.Context
	cancel           context.CancelFunc
	wg               = new(sync.WaitGroup)
	send             = flag.Bool("s", false, "Data Object Send")
	max_ct           = flag.Uint("count", 4, "send count messages to receiver")
	size             = flag.Int("size", 460000, "message size in bytes")
	recv             = flag.Bool("r", false, "Data Object Receive")
	trace            = flag.Bool("trace", false, "enable session message trace")
	debug_instance   = flag.Bool("di", false, "debug norm.Instance, not NormSetDebug()")
	norm_debug_level = flag.Uint("d", 0, "NormSetDebugLevel(): 0-12")
	debug_log        = flag.String("dl", "", "debug log file name: NormOpenDebugLog()")
	s_tx_rate        = flag.Float64("stxrate", 0, "tx_rate, use cc when 0")
	s_tx_rate_min    = flag.Float64("stxratemin", 0, "set_tx_rate_bounds: rate_min")
	s_tx_rate_max    = flag.Float64("stxratemax", 0, "set_tx_rate_bounds: rate_max")
	s_addr           = flag.String("saddr", "127.0.0.1", "sender session address")
	s_port           = flag.Uint("sport", 6003, "sender session port")
	s_node_id        = flag.Uint("snodeid", 6, "sender node id")
	r_addr           = flag.String("raddr", "0.0.0.0", "receiver session address")
	r_port           = flag.Uint("rport", 6003, "receiver session port")
	r_node_id        = flag.Uint("rnodeid", 7, "receiver node id")
)

func do_send() {
	defer wg.Done()
	defer cancel()
	i, err := norm.Create_instance(wg)
	if err != nil {
		log.Println(err)
		return
	}
	defer i.Destroy()
	if 0 < len(*debug_log) {
		log.Println("debug log:", *debug_log)
		i.Open_debug_log(*debug_log)
		defer i.Close_debug_log()
	}
	log.Println("norm version:", i.Get_version())
	old := i.Get_debug_level()
	i.Set_debug_level(*norm_debug_level)
	log.Printf("norm debug level: %v -> %v\n", old, *norm_debug_level)
	i.Set_debug(*debug_instance)
	log.Printf("sender session: %v:%v\n", *s_addr, *s_port)
	sess, err := i.Create_session(*s_addr, uint16(*s_port), norm.Node_id((*s_node_id)))
	if err != nil {
		log.Println(err)
		return
	}
	log.Println("id:", sess.Get_local_node_id())
	log.Println("trace:", *trace)
	sess.Set_message_trace(*trace)
	sess.Set_backoff_factor(2.0)
	sess.Set_group_size(10)
	sess.Set_tx_only(true, false)
	sess.Set_events(norm.Event_type_all &^ (norm.Event_type_grtt_updated | norm.Event_type_send_error | norm.Event_type_tx_rate_changed))
	defer sess.Destroy()
	if err = sess.Add_acking_node(norm.Node_id(*r_node_id)); err != nil {
		log.Println("Add_acking_node", err)
		return
	}
	sess.Set_tx_cache_bounds(1e+6*100, 1000, 2000)
	switch {
	case 0 < *s_tx_rate:
		log.Printf("set_tx_rate: %.f\n", *s_tx_rate)
		sess.Set_tx_rate(*s_tx_rate)
	case 0 < *s_tx_rate_min || 0 < *s_tx_rate_max:
		log.Println("setting cc: true, true")
		sess.Set_congestion_control(true, true)
		log.Printf("Set_tx_rate_bounds: rate_min: %.f, rate_max: %.f\n", *s_tx_rate_min, *s_tx_rate_max)
		sess.Set_tx_rate_bounds(*s_tx_rate_min, *s_tx_rate_max)
	default:
		log.Println("setting cc: true, true")
		sess.Set_congestion_control(true, true)
	}
	if err := sess.Start_sender(0, 1024*1024*4, 0, 0, 0, 0); err != nil {
		log.Println(err)
		return
	}
	defer sess.Stop_sender()
	ct := uint(1)
	obj, err := sess.Data_enqueue(
		bytes.Repeat([]byte{'a'}, int(*size)),
		[]byte(fmt.Sprintf("Data, count: %v of %v, size: %v", ct, *max_ct, *size)),
	)
	if err != nil {
		log.Println(err)
		return
	}
	log.Printf("data enqueue: %v of %v\n", ct, *max_ct)
	ct++
	cmd := []byte("Command is Hello!")
	if !sess.Send_command(cmd, false) {
		log.Println("tx command failed")
		return
	}
	log.Println("tx cmd:", string(cmd))
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
				if ct <= *max_ct {
					obj, err = sess.Data_enqueue(
						bytes.Repeat([]byte{'a'}, int(*size)),
						[]byte(fmt.Sprintf("Data, count: %v of %v, size: %v", ct, *max_ct, *size)),
					)
					if err != nil {
						log.Println(err)
						return
					}
					log.Printf("data enqueue: %v of %v\n", ct, *max_ct)
					ct++
				} else {
					if obj != nil {
						sess.Set_watermark(obj, true)
						obj = nil
					}
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
	defer i.Destroy()
	if 0 < len(*debug_log) {
		log.Println("debug log:", *debug_log)
		i.Open_debug_log(*debug_log)
		defer i.Close_debug_log()
	}
	old := i.Get_debug_level()
	i.Set_debug_level(*norm_debug_level)
	log.Printf("norm debug level: %v -> %v\n", old, *norm_debug_level)
	i.Set_debug(*debug_instance)
	log.Printf("receiver session: %v:%v\n", *r_addr, *r_port)
	sess, err := i.Create_session(*r_addr, uint16(*r_port), norm.Node_id(*r_node_id))
	if err != nil {
		log.Println(err)
	}
	defer sess.Destroy()
	log.Println("id:", sess.Get_local_node_id())
	sess.Set_message_trace(*trace)
	sess.Set_events(norm.Event_type_all &^ (norm.Event_type_grtt_updated | norm.Event_type_rx_object_updated | norm.Event_type_rx_object_updated))
	sess.Set_default_unicast_nack(true)
	sess.Set_rx_cache_limit(4096)
	if err = sess.Start_receiver(1024 * 1024); err != nil {
		log.Println(err)
		return
	}
	defer sess.Stop_receiver()
	if !sess.Set_rx_socket_buffer(4 * 1024 * 1024) {
		log.Println("Set_rx_socket_buffer failed")
		return
	}
	var start_time time.Time
	for {
		select {
		case e := <-sess.Events():
			switch e.Type {
			case norm.Event_type_remote_sender_new, norm.Event_type_remote_sender_active, norm.Event_type_remote_sender_inactive:
				log.Println("main:", e, e.Node.Get_id(), e.Node.Get_address_all())
			case norm.Event_type_rx_cmd_new:
				log.Println(e, "rx command:", e.Node.Get_command())
			case norm.Event_type_rx_object_new:
				start_time = time.Now()
			case norm.Event_type_rx_object_info:
				log.Printf("main: %v, info: %v ", e, e.O.Get_info())
			case norm.Event_type_rx_object_updated:
				ocompleted := e.O.Get_size() - e.O.Get_bytes_pending()
				percent := 100.00 * (float32(ocompleted) / float32(e.O.Get_size()))
				log.Printf("%v: Id: %v, completion status %v/%v %6.2f%%\n", flag.Args()[0], e.O.Id, ocompleted, e.O.Get_size(), percent)
			case norm.Event_type_rx_object_completed:
				duration := time.Now().Sub(start_time).Seconds()
				data := e.O.Data_access_data(true)
				log.Printf("id: %v, transfer duration: %.6f sec at %.6f kbps\n",
					e.O.Id,
					time.Now().Sub(start_time).Seconds(),
					8.0/1000.0*float64(data.Len())/duration,
				)
				log.Println("main:", e, "data size:", data.Len())
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
	flag.Parse()
	switch {
	case (!*send && !*recv) || (*send && *recv):
		flag.PrintDefaults()
		return
	case *send:
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

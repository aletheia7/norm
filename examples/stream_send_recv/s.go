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
	"strconv"
	"strings"
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
	s_addr           = flag.String("saddr", "127.0.0.1", "sender session address")
	s_port           = flag.Int("sport", 6003, "sender session port")
	s_node_id        = flag.Uint("snodeid", 6, "sender node id")
	r_addr           = flag.String("raddr", "0.0.0.0", "receiver session address")
	r_port           = flag.Int("rport", 6003, "receiver session port")
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
	i.Set_debug(*debug_instance)
	defer i.Destroy()
	i.Set_debug_level(1)
	log.Printf("sender session: %v:%v\n", *s_addr, *s_port)
	sess, err := i.Create_session(*s_addr, *s_port, norm.Node_id(*s_node_id))
	if err != nil {
		log.Println(err)
		return
	}
	sess.Set_message_trace(*trace)
	sess.Set_tx_only(true, false)
	sess.Set_events(norm.Event_type_all &^ (norm.Event_type_grtt_updated | norm.Event_type_tx_rate_changed))
	defer sess.Destroy()
	if !sess.Add_acking_node(2) {
		log.Println("Add_acking_node", err)
		return
	}
	// sess.Set_tx_cache_bounds(100e+6, 1000, 2000)
	sess.Set_congestion_control(true, true)
	if !sess.Start_sender(0, 0, 0, 0, 0, 0) {
		log.Println(err)
		return
	}
	defer sess.Stop_sender()
	info := bytes.NewBufferString(fmt.Sprintf("stream size: %v", *size))
	obj, err := sess.Stream_open(4*1024*1024, info.Bytes())
	if err != nil {
		log.Println(err)
		return
	}
	data := bytes.NewBuffer(bytes.Repeat([]byte{'a'}, *size))
	ct := uint(1)
	if obj.Stream_write(data.Bytes()) == data.Len() {
		log.Println("stream_write:", ct, "of", *max_ct)
		obj.Stream_flush(true, norm.Flush_active)
	} else {
		log.Println("stream_write failed:", i)
		return
	}
	ct++
	for {
		select {
		case e := <-sess.Events():
			switch e.Type {
			case norm.Event_type_tx_queue_empty, norm.Event_type_tx_queue_vacancy:
				log.Println("main:", e)
				if ct > *max_ct {
					continue
				}
				data = bytes.NewBuffer(bytes.Repeat([]byte{'a'}, *size))
				if obj.Stream_write(data.Bytes()) == data.Len() {
					log.Println("stream_write:", ct, "of", *max_ct)
					obj.Stream_flush(true, norm.Flush_active)
				} else {
					log.Println("bytes != full:", i)
				}
				ct++
			case norm.Event_type_tx_watermark_completed:
				log.Println(e)
			case norm.Event_type_tx_flush_completed:
				log.Println("main:", e)
				if obj != nil {
					obj.Stream_close(true)
					obj = nil
				}
			case norm.Event_type_send_error:
				log.Println(e)
			case norm.Event_type_grtt_updated, norm.Event_type_tx_object_sent,
				norm.Event_type_tx_object_purged:
				log.Println(e)
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
	i.Set_debug(*debug_instance)
	defer i.Destroy()
	i.Set_debug_level(1)
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
	if !sess.Start_receiver(8 * 1024 * 1024) {
		log.Println(err)
		return
	}
	defer sess.Stop_receiver()
	var start_time time.Time
	stream_sync := false
	var obj *norm.Object
	var read_bytes bytes.Buffer
	canceled := true
	var msize uint
	for {
		select {
		case e := <-sess.Events():
			switch e.Type {
			case norm.Event_type_remote_sender_new:
				log.Println("main:", e)
				log.Println("sender id:", e.Node.Get_id())
				addr, port, _ := e.Node.Get_address()
				log.Printf("sender %v:%v\n", addr, port)
			case norm.Event_type_rx_object_new:
				if e.O.Get_type() == norm.Object_type_stream && start_time.IsZero() {
					start_time = time.Now()
					obj = e.O
				}
			case norm.Event_type_rx_object_info:
				a := strings.Split(e.O.Get_info().String(), " ")
				i, _ := strconv.Atoi(a[len(a)-1])
				msize = uint(i)
				log.Println("main:", e, e.O.Get_info())
				obj = e.O
			case norm.Event_type_rx_object_updated:
				if e.O.Get_type() == norm.Object_type_stream && start_time.IsZero() {
					start_time = time.Now()
					obj = e.O
				}
				if !stream_sync {
					if stream_sync = obj.Stream_seek_msg_start(); !stream_sync {
						continue
					}
				}
				data, _ := obj.Stream_read()
				_, err := read_bytes.Write(data.Bytes())
				if err != nil {
					log.Println(err)
					return
				}
				if !canceled && 50000 < read_bytes.Len() {
					canceled = true
					obj.Cancel()
					log.Println("main: obj canceled:", obj)
				}
				if int(msize) <= read_bytes.Len() {
					duration := time.Now().Sub(start_time).Seconds()
					log.Printf("id: %v, transfer duration: %.6f sec at %.6f kbps\n",
						e.O.Id,
						time.Now().Sub(start_time).Seconds(),
						8.0/1000.0*float64(int(msize))/duration,
					)
					read_bytes.Next(int(msize))
					start_time = time.Time{}
				}
			default:
				log.Println("main:", e)
			}
		case <-ctx.Done():
			return
		}
	}
}

func main() {
	log.SetFlags(log.Lmicroseconds | log.Ltime | log.Lshortfile)
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

// Copyright 2016 aletheia7. All rights reserved. Use of this source code is
// governed by a BSD-2-Clause license that can be found in the LICENSE file.

// +build linux,cgo,amd64 linux,cgo,386

package norm

/*
#cgo CFLAGS:  -I${SRCDIR}/norm/include -I${SRCDIR}/protolib/include
#cgo LDFLAGS: -L${SRCDIR}/norm/lib -lnorm -L${SRCDIR}/norm/protolib/lib  -lprotokit -lpthread -lstdc++ -lm
#include <stdlib.h>
#include "normApi.h"
*/
import "C"

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"syscall"
	"unsafe"
)

const (
	event_type_invalid                Event_type = Event_type(1) << C.NORM_EVENT_INVALID
	event_type_tx_queue_vacancy                  = Event_type(1) << C.NORM_TX_QUEUE_VACANCY
	event_type_tx_queue_empty                    = Event_type(1) << C.NORM_TX_QUEUE_EMPTY
	event_type_tx_flush_completed                = Event_type(1) << C.NORM_TX_FLUSH_COMPLETED
	event_type_tx_watermark_completed            = Event_type(1) << C.NORM_TX_WATERMARK_COMPLETED
	event_type_tx_cmd_sent                       = Event_type(1) << C.NORM_TX_CMD_SENT
	event_type_tx_object_sent                    = Event_type(1) << C.NORM_TX_OBJECT_SENT
	event_type_tx_object_purged                  = Event_type(1) << C.NORM_TX_OBJECT_PURGED
	event_type_tx_rate_changed                   = Event_type(1) << C.NORM_TX_RATE_CHANGED
	event_type_local_sender_closed               = Event_type(1) << C.NORM_LOCAL_SENDER_CLOSED
	event_type_remote_sender_new                 = Event_type(1) << C.NORM_REMOTE_SENDER_NEW
	event_type_remote_sender_reset               = Event_type(1) << C.NORM_REMOTE_SENDER_RESET   // remote sender instanceId or FEC params changed
	event_type_remote_sender_address             = Event_type(1) << C.NORM_REMOTE_SENDER_ADDRESS // remote sender src addr and/or port changed
	event_type_remote_sender_active              = Event_type(1) << C.NORM_REMOTE_SENDER_ACTIVE
	event_type_remote_sender_inactive            = Event_type(1) << C.NORM_REMOTE_SENDER_INACTIVE
	event_type_remote_sender_purged              = Event_type(1) << C.NORM_REMOTE_SENDER_PURGED // not yet implemented
	event_type_rx_cmd_new                        = Event_type(1) << C.NORM_RX_CMD_NEW
	event_type_rx_object_new                     = Event_type(1) << C.NORM_RX_OBJECT_NEW
	event_type_rx_object_info                    = Event_type(1) << C.NORM_RX_OBJECT_INFO
	event_type_rx_object_updated                 = Event_type(1) << C.NORM_RX_OBJECT_UPDATED
	event_type_rx_object_completed               = Event_type(1) << C.NORM_RX_OBJECT_COMPLETED
	event_type_rx_object_aborted                 = Event_type(1) << C.NORM_RX_OBJECT_ABORTED
	event_type_grtt_updated                      = Event_type(1) << C.NORM_GRTT_UPDATED
	event_type_cc_active                         = Event_type(1) << C.NORM_CC_ACTIVE
	event_type_cc_inactive                       = Event_type(1) << C.NORM_CC_INACTIVE
	event_type_acking_node_new                   = Event_type(1) << C.NORM_ACKING_NODE_NEW // NormSetAutoAcking
	event_type_send_error                        = Event_type(1) << C.NORM_SEND_ERROR      // ICMP error (e.g. destination unreachable)
	event_type_user_timeout                      = Event_type(1) << C.NORM_USER_TIMEOUT    // issues when timeout set by NormSetUserTimer() expires
	event_type_all                               = ^Event_type(0)
)

const (
	object_type_none   Object_type = Object_type(C.NORM_OBJECT_NONE)
	object_type_data               = Object_type(C.NORM_OBJECT_DATA)
	object_type_file               = Object_type(C.NORM_OBJECT_FILE)
	object_type_stream             = Object_type(C.NORM_OBJECT_STREAM)
)

const (
	ack_invalid = Acking_status(C.NORM_ACK_INVALID)
	ack_failure = Acking_status(C.NORM_ACK_FAILURE)
	ack_pending = Acking_status(C.NORM_ACK_PENDING)
	ack_success = Acking_status(C.NORM_ACK_SUCCESS)
)

var (
	node_none = Node_id(C.NORM_NODE_NONE)
	node_any  = Node_id(C.NORM_NODE_ANY)
)

const (
	sync_current = Sync_policy(C.NORM_SYNC_CURRENT)
	sync_all     = Sync_policy(C.NORM_SYNC_ALL)
)

const (
	nack_none      = Nacking_mode(C.NORM_NACK_NONE)
	nack_info_only = Nacking_mode(C.NORM_NACK_INFO_ONLY)
	nack_normal    = Nacking_mode(C.NORM_NACK_NORMAL)
)

const (
	repair_block  = Repair_boundary(C.NORM_BOUNDARY_BLOCK)
	repair_object = Repair_boundary(C.NORM_BOUNDARY_OBJECT)
)

var instance_lock sync.Mutex

func create_instance(wg *sync.WaitGroup) (r *Instance, err error) {
	r = &Instance{
		wg:      wg,
		sess_db: refdb{},
		ctx:     context.Background(),
		api:     make(chan apier, 200),
	}
	r.cmd_mgr = new_cmd_mgr(nil)
	r.cmdc = make(chan interface{}, 200)
	r.lock.Lock()
	defer r.lock.Unlock()
	if r.handle = norm_instance_handle(C.NormCreateInstance(false)); r.handle == norm_instance_handle(C.NORM_INSTANCE_INVALID) {
		return nil, errors.New("instance invalid")
	}
	wg.Add(1)
	go r.loop()
	return
}

type (
	norm_instance_handle C.NormInstanceHandle
	norm_event           C.NormEvent
	norm_session_handle  C.NormSessionHandle
	norm_node_id         C.NormNodeId
	norm_session_id      C.NormSessionId
	norm_object_handle   C.NormObjectHandle
	norm_node_handle     C.NormNodeHandle
	norm_acking_status   C.NormAckingStatus
	norm_sync_policy     C.NormSyncPolicy
	norm_nacking_mode    C.NormNackingMode
	norm_repair_boundary C.NormRepairBoundary
	p_c_char             *C.char
	c_uint               C.uint
)

type icmd uint8

const (
	icmd_unknown icmd = iota
	icmd_stop_instance
	icmd_restart_instance
	icmd_destroy_instance
	icmd_create_session
	icmd_destroy_session
	icmd_start_sender
	icmd_stop_sender
	icmd_data_enqueue
	icmd_object_requeue
	icmd_set_loopback
	icmd_set_default_unicast_nack
	icmd_set_tx_port
	icmd_set_rx_port_reuse
	icmd_set_rx_cache_limit
	icmd_start_receiver
	icmd_stop_receiver
	icmd_add_acking_node
	icmd_remove_acking_node
	icmd_get_next_acking_node
	icmd_get_acking_status
	icmd_set_tx_cache_bounds
	icmd_set_congestion_control
	icmd_set_tx_only
	icmd_set_debug_level
	icmd_open_debug_log
	icmd_close_debug_log
	icmd_open_debug_pipe
	icmd_close_debug_pipe
	icmd_set_message_trace
	icmd_get_debug_level
	icmd_stream_open
	icmd_stream_close
	icmd_stream_mark_eom
	icmd_stream_seek_msg_start
	icmd_stream_get_read_offset
	icmd_stream_write
	icmd_stream_flush
	icmd_stream_read
	icmd_set_watermark
	icmd_cancel_watermark
	icmd_object_cancel
	icmd_node_get_id
	icmd_node_get_address
	icmd_node_get_command
	icmd_get_version
	icmd_file_enqueue
	icmd_set_cache_directory
	icmd_send_command
	icmd_cancel_command
	icmd_object_get_info
	icmd_object_get_size
	icmd_object_get_bytes_pending
	icmd_object_retain
	icmd_object_release
	icmd_file_get_name
	icmd_file_rename
	icmd_data_access_data
	icmd_object_get_sender
	icmd_get_tx_rate
	icmd_set_tx_rate
	icmd_set_tx_rate_bounds
	icmd_set_tx_socket_buffer
	icmd_set_rx_socket_buffer
	icmd_get_local_node_id
	icmd_set_multicast_interface
	icmd_set_ssm
	icmd_set_ttl
	icmd_set_tos
	icmd_set_fragmentation
	icmd_set_flow_control
	icmd_set_auto_parity
	icmd_get_grtt_estimate
	icmd_set_grtt_estimate
	icmd_set_grtt_max
	icmd_set_grtt_probing_mode
	icmd_set_grtt_probing_interval
	icmd_set_backoff_factor
	icmd_set_group_size
	icmd_set_tx_robust_factor
	icmd_stream_set_auto_flush
	icmd_stream_set_push_enable
	icmd_stream_has_vacancy
	icmd_set_silent_receiver
	icmd_node_set_unicast_nack
	icmd_set_default_sync_policy
	icmd_set_default_nacking_mode
	icmd_node_set_nacking_mode
	icmd_object_set_nacking_mode
	icmd_set_default_repair_boundary
	icmd_node_set_repair_boundary
	icmd_set_default_rx_robust_factor
	icmd_object_has_info
	icmd_object_get_info_length
	icmd_node_get_grtt
	icmd_node_free_buffers
	icmd_node_delete
	icmd_node_retain
	icmd_node_release
)

var icmds = map[icmd]string{
	icmd_unknown:                      "unknown",
	icmd_stop_instance:                "stop instance",
	icmd_restart_instance:             "restart instance",
	icmd_destroy_instance:             "destroy instance",
	icmd_create_session:               "create session",
	icmd_destroy_session:              "destroy_session",
	icmd_start_sender:                 "start sender",
	icmd_stop_sender:                  "stop sender",
	icmd_data_enqueue:                 "data enqueue",
	icmd_object_requeue:               "object requeue",
	icmd_set_loopback:                 "set loopback",
	icmd_set_default_unicast_nack:     "set default unicast nack",
	icmd_set_tx_port:                  "set tx port",
	icmd_set_rx_port_reuse:            "set rx port reuse",
	icmd_set_rx_cache_limit:           "set rx cache limit",
	icmd_start_receiver:               "start receiver",
	icmd_stop_receiver:                "stop receiver",
	icmd_add_acking_node:              "add acking node",
	icmd_remove_acking_node:           "remove acking node",
	icmd_get_next_acking_node:         "get next acking node",
	icmd_get_acking_status:            "get acking status",
	icmd_set_tx_cache_bounds:          "set tx cache bounds",
	icmd_set_congestion_control:       "set congestion control",
	icmd_set_tx_only:                  "set tx only",
	icmd_set_debug_level:              "set debug level",
	icmd_open_debug_log:               "open debug log",
	icmd_close_debug_log:              "close debug log",
	icmd_open_debug_pipe:              "open debug pipe",
	icmd_close_debug_pipe:             "clsoe debug pipe",
	icmd_set_message_trace:            "set message trace",
	icmd_get_debug_level:              "get debug level",
	icmd_stream_open:                  "stream open",
	icmd_stream_close:                 "stream close",
	icmd_stream_mark_eom:              "stream mark eom",
	icmd_stream_seek_msg_start:        "stream seek msg start",
	icmd_stream_get_read_offset:       "stream get read offset",
	icmd_stream_write:                 "stream write",
	icmd_stream_read:                  "stream read",
	icmd_stream_flush:                 "stream flush",
	icmd_set_watermark:                "set watermark",
	icmd_cancel_watermark:             "cancel watermark",
	icmd_object_cancel:                "object cancel",
	icmd_node_get_id:                  "node get id",
	icmd_node_get_address:             "node get address",
	icmd_node_get_command:             "node get command",
	icmd_get_version:                  "get version",
	icmd_file_enqueue:                 "file enqueue",
	icmd_set_cache_directory:          "set cache directory",
	icmd_send_command:                 "send command",
	icmd_cancel_command:               "cancel command",
	icmd_object_get_info:              "object get info",
	icmd_object_get_size:              "object get size",
	icmd_object_get_bytes_pending:     "object get bytes pending",
	icmd_object_retain:                "object retain",
	icmd_object_release:               "object release",
	icmd_file_get_name:                "file get name",
	icmd_file_rename:                  "file rename",
	icmd_data_access_data:             "data access data",
	icmd_object_get_sender:            "object get sender",
	icmd_get_tx_rate:                  "get tx rate",
	icmd_set_tx_rate:                  "set_tx_rate",
	icmd_set_tx_rate_bounds:           "set tx rate bounds",
	icmd_set_tx_socket_buffer:         "set tx socket buffer",
	icmd_set_rx_socket_buffer:         "set rx socket buffer",
	icmd_get_local_node_id:            "get local norm id",
	icmd_set_multicast_interface:      "set multicast interface",
	icmd_set_ssm:                      "set ssm",
	icmd_set_ttl:                      "set ttl",
	icmd_set_tos:                      "set tos",
	icmd_set_fragmentation:            "set fragmentation",
	icmd_set_flow_control:             "set flow control",
	icmd_set_auto_parity:              "set auto parity",
	icmd_get_grtt_estimate:            "get grtt estimate",
	icmd_set_grtt_estimate:            "set grtt estiamte",
	icmd_set_grtt_max:                 "set grtt max",
	icmd_set_grtt_probing_mode:        "set grtt probing mode",
	icmd_set_grtt_probing_interval:    "set grtt probing interval",
	icmd_set_backoff_factor:           "set backoff factor",
	icmd_set_group_size:               "set group size",
	icmd_set_tx_robust_factor:         "set tx robust factor",
	icmd_stream_set_auto_flush:        "stream set auto flush",
	icmd_stream_set_push_enable:       "stream set push enable",
	icmd_stream_has_vacancy:           "stream has vacancy",
	icmd_set_silent_receiver:          "set silent receiver",
	icmd_node_set_unicast_nack:        "node set unicast nack",
	icmd_set_default_sync_policy:      "set default sync policy",
	icmd_set_default_nacking_mode:     "set default nacking mode",
	icmd_node_set_nacking_mode:        "node set nacking mode",
	icmd_object_set_nacking_mode:      "object set nacking mode",
	icmd_set_default_repair_boundary:  "set default repair boundary",
	icmd_node_set_repair_boundary:     "node set repair boundary",
	icmd_set_default_rx_robust_factor: "set default rx robust factor",
	icmd_object_has_info:              "object has info",
	icmd_object_get_info_length:       "object get info length",
	icmd_node_get_grtt:                "node get grtt",
	icmd_node_free_buffers:            "node free buffers",
	icmd_node_delete:                  "node delete",
	icmd_node_retain:                  "node retain",
	icmd_node_release:                 "node release",
}

func (o icmd) String() string {
	if s, ok := icmds[o]; ok {
		return s
	}
	panic(fmt.Sprintf("missing icmd: %v", o))
}

const (
	flush_none    Flush_mode = Flush_mode(C.NORM_FLUSH_NONE)
	flush_passive            = Flush_mode(C.NORM_FLUSH_PASSIVE)
	flush_active             = Flush_mode(C.NORM_FLUSH_ACTIVE)
)

const (
	probe_none    Probe_mode = Probe_mode(C.NORM_PROBE_NONE)
	probe_passive            = Probe_mode(C.NORM_PROBE_PASSIVE)
	probe_active             = Probe_mode(C.NORM_PROBE_ACTIVE)
)

type address struct {
	p *C.char
}

func new_address(s string) (r *address) {
	r = &address{}
	if 0 < len(s) {
		r.p = C.CString(s)
	}
	return
}

func (o *address) free() {
	if o.p != nil {
		C.free(unsafe.Pointer(o.p))
		o.p = nil
	}
}

func b2c(o *bytes.Buffer) *C.char {
	if o.Len() == 0 {
		return (*C.char)(unsafe.Pointer(nil))
	} else {
		return (*C.char)(unsafe.Pointer(&o.Bytes()[0]))
	}
}

func b2UINT32(o *bytes.Buffer) C.UINT32 {
	return C.UINT32(uint32(o.Len()))
}

func b2UINT16(o *bytes.Buffer) C.UINT16 {
	return C.UINT16(uint16(o.Len()))
}

func b2uint(o *bytes.Buffer) C.uint {
	return (C.uint)(o.Len())
}

type refdb map[norm_session_handle]*Session

func (o refdb) add(sess *Session) {
	if _, ok := o[sess.handle]; ok {
		panic("session already addeed to refdb")
	} else {
		o[sess.handle] = sess
	}
}

func (o refdb) remove(sess *Session) {
	if _, ok := o[sess.handle]; ok {
		delete(o, sess.handle)
	} else {
		panic("session doesn't exist in refdb")
	}
}

func (o session_start_sender) call() {
	if !C.NormStartSender(o.sess.handle, C.NormSessionId(o.id), C.UINT32(o.buffer_space), C.UINT16(o.segment_size), C.UINT16(o.block_size), C.UINT16(o.num_parity), C.UINT8(o.fec_id)) {
		o.err = E_false
	}
	if o.call_done == nil {
		panic("call_done is nil")
	}
	o.call_done <- struct{}{}
}

func (o *Instance) loop() {
	defer o.wg.Done()
	var desc *os.File
	norm_descriptor := C.NormGetDescriptor(o.handle)
	if norm_descriptor == C.NORM_DESCRIPTOR_INVALID {
		return
	}
	desc = os.NewFile(uintptr(norm_descriptor), "norm descriptor")
	shutdown_r, shutdown_w, err := os.Pipe()
	if err != nil {
		return
	}
	if err = syscall.SetNonblock(int(shutdown_r.Fd()), true); err != nil {
		return
	}
	if err = syscall.SetNonblock(int(shutdown_w.Fd()), true); err != nil {
		return
	}
	var (
		desc_ready          = make(chan struct{}, 1)
		desc_ready_received = make(chan struct{}, 1)
	)
	o.wg.Add(1)
	go o.descriptor_ready(desc, desc_ready, desc_ready_received, shutdown_r)
	desc_ready_received <- struct{}{}
	defer func() {
		close(desc_ready_received)
		close(desc_ready)
		shutdown_w.Close()
		syscall.Close(int(desc.Fd()))
		C.NormDestroyInstance(o.handle)
		if 0 < len(o.sess_db) {
			E_sender_sessions_not_destroyed = fmt.Errorf("sender sessions not destroyed: %v", len(o.sess_db))
			o.errc <- E_sender_sessions_not_destroyed
		} else {
			o.errc <- nil
		}
	}()
	for {
		select {
		case a := <-o.api:
			log.Printf("%#v\n", a)
			a.call()
		case i := <-o.cmdc:
			switch t := i.(type) {
			case icmd:
				if o.debug {
					log.Println("icmd", t)
				}
				switch t {
				case icmd_destroy_instance:
					if o.debug {
						log.Println("id:", o.object_id_ct)
					}
					return
				case icmd_set_debug_level:
					C.NormSetDebugLevel(C.uint(o.debug_level))
					o.errc <- nil
				case icmd_open_debug_log:
					fn := C.CString(o.file_name)
					ret := C.NormOpenDebugLog(o.handle, fn)
					C.free(unsafe.Pointer(fn))
					if ret {
						o.errc <- nil
					} else {
						o.errc <- E_false
					}
				case icmd_close_debug_log:
					C.NormCloseDebugLog(o.handle)
					o.errc <- nil
				case icmd_open_debug_pipe:
					fn := C.CString(o.file_name)
					ret := C.NormOpenDebugPipe(o.handle, fn)
					C.free(unsafe.Pointer(fn))
					if ret {
						o.errc <- nil
					} else {
						o.errc <- E_false
					}
				case icmd_close_debug_pipe:
					C.NormCloseDebugPipe(o.handle)
					o.errc <- nil
				case icmd_get_debug_level:
					o.debug_level = uint(C.NormGetDebugLevel())
					o.errc <- nil
				case icmd_get_version:
					var major, minor, patch C.int
					C.NormGetVersion(&major, &minor, &patch)
					o.version = fmt.Sprintf("%v.%v.%v", major, minor, patch)
					o.errc <- nil
				case icmd_set_cache_directory:
					cp := C.CString(o.cache_path)
					ret := bool(C.NormSetCacheDirectory(o.handle, cp))
					C.free(unsafe.Pointer(cp))
					if ret {
						o.errc <- nil
					} else {
						o.errc <- E_false
					}
				case icmd_stop_instance:
					C.NormStopInstance(o.handle)
					o.errc <- nil
				case icmd_restart_instance:
					if bool(C.NormRestartInstance(o.handle)) {
						o.errc <- nil
					} else {
						o.errc <- E_false
					}
				}
			case *Session:
				if o.debug {
					log.Println("icmd", t.cmd)
				}
				switch t.cmd {
				case icmd_data_enqueue:
					if t.obj.handle = norm_object_handle(C.NormDataEnqueue(t.handle, b2c(t.obj.data), b2UINT32(t.obj.data), b2c(t.obj.info), b2uint(t.obj.info))); t.obj.handle == norm_object_handle(C.NORM_OBJECT_INVALID) {
						t.errc <- E_object_invalid
					} else {
						o.object_id_ct++
						t.obj.Id = o.object_id_ct
						t.objects[t.obj.handle] = t.obj
						t.errc <- nil
					}
				case icmd_set_watermark:
					if bool(C.NormSetWatermark(t.handle, t.obj.handle, C._Bool(t.enable))) {
						t.errc <- nil
					} else {
						t.errc <- E_false
					}
				case icmd_cancel_watermark:
					// todo_brian: sig diff
					C.NormCancelWatermark(t.handle)
					t.errc <- nil
				case icmd_stream_open:
					if t.obj.handle = norm_object_handle(C.NormStreamOpen(t.handle, C.UINT32(t.size), b2c(t.obj.info), b2uint(t.obj.info))); t.obj.handle == norm_object_handle(C.NORM_OBJECT_INVALID) {
						t.errc <- E_object_invalid
					} else {
						o.object_id_ct++
						t.obj.Id = o.object_id_ct
						t.objects[t.obj.handle] = t.obj
						t.errc <- nil
					}
				case icmd_create_session:
					ba := new_address(t.bind_address)
					t.handle = norm_session_handle(C.NormCreateSession(o.handle, ba.p, C.UINT16(t.port), C.NormNodeId(t.node_id)))
					ba.free()
					if t.handle == norm_session_handle(C.NORM_SESSION_INVALID) {
						t.errc <- E_session_invalid
					} else {
						o.sess_db.add(t)
						t.errc <- nil
					}
				case icmd_destroy_session:
					if o.debug {
						log.Println("session objects:", len(o.sess_db[t.handle].objects))
					}
					C.NormDestroySession(t.handle)
					o.sess_db.remove(t)
					t.errc <- nil
				case icmd_add_acking_node:
					if C.NormAddAckingNode(t.handle, C.NormNodeId(t.node_id)) {
						t.errc <- nil
					} else {
						t.errc <- E_false
					}
				case icmd_remove_acking_node:
					C.NormRemoveAckingNode(t.handle, C.NormNodeId(t.node_id))
					t.errc <- nil
				case icmd_get_next_acking_node:
					// todo_brian: sig diff
					if bool(C.NormGetNextAckingNode(t.handle, (*C.NormNodeId)(&t.node_id), (*C.NormAckingStatus)(&t.acking_status))) {
						t.errc <- nil
					} else {
						t.errc <- E_false
					}
				case icmd_get_acking_status:
					t.acking_status = Acking_status(C.NormGetAckingStatus(t.handle, C.NormNodeId(t.node_id)))
					t.errc <- nil
				case icmd_set_tx_cache_bounds:
					C.NormSetTxCacheBounds(t.handle, C.NormSize(t.tx_cache_bounds_size_max), C.UINT32(t.tx_cache_bounds_count_min), C.UINT32(t.tx_cache_bounds_count_max))
					t.errc <- nil
				case icmd_set_congestion_control:
					C.NormSetCongestionControl(t.handle, C._Bool(t.enable), C._Bool(t.enable2))
					t.errc <- nil
				case icmd_set_tx_only:
					C.NormSetTxOnly(t.handle, C._Bool(t.enable), C._Bool(t.enable2))
					t.errc <- nil
				case icmd_set_tx_port:
					ba := new_address(t.bind_address)
					if C.NormSetTxPort(t.handle, C.UINT16(t.port), C._Bool(t.enable), ba.p) {
						ba.free()
						t.errc <- nil
					} else {
						ba.free()
						t.errc <- E_false
					}
				case icmd_set_rx_port_reuse:
					ba := new_address(t.bind_address)
					sa := new_address(t.sender_address)
					C.NormSetRxPortReuse(t.handle, C._Bool(t.enable), ba.p, sa.p, (C.UINT16)(t.port))
					ba.free()
					sa.free()
					t.errc <- nil
				case icmd_object_requeue:
					if C.NormRequeueObject(t.handle, t.obj.handle) {
						t.errc <- nil
					} else {
						t.errc <- E_false
					}
				// case icmd_start_sender:
				// 	if C.NormStartSender(t.handle, C.NormSessionId(t.id), C.UINT32(t.size), C.UINT16(t.segment_size), C.UINT16(t.block_size), C.UINT16(t.num_parity), C.UINT8(t.u8)) {
				// 		t.errc <- nil
				// 	} else {
				// 		t.errc <- E_false
				// 	}
				case icmd_stop_sender:
					C.NormStopSender(t.handle)
					t.release_all()
					t.errc <- nil
				case icmd_set_loopback:
					C.NormSetLoopback(t.handle, C._Bool(t.enable))
					t.errc <- nil
				case icmd_start_receiver:
					if C.NormStartReceiver(t.handle, C.UINT32(t.size)) {
						t.errc <- nil
					} else {
						t.errc <- E_false
					}
				case icmd_stop_receiver:
					C.NormStopReceiver(t.handle)
					t.release_all()
					t.errc <- nil
				case icmd_set_default_unicast_nack:
					C.NormSetDefaultUnicastNack(t.handle, C._Bool(t.enable))
					t.errc <- nil
				case icmd_set_rx_cache_limit:
					C.NormSetRxCacheLimit(t.handle, C.ushort(t.i))
					t.errc <- nil
				case icmd_set_message_trace:
					C.NormSetMessageTrace(t.handle, C._Bool(t.enable))
					t.errc <- nil
				case icmd_file_enqueue:
					fn := C.CString(t.file_name)
					t.obj.handle = norm_object_handle(C.NormFileEnqueue(t.handle, fn, b2c(t.obj.info), b2uint(t.obj.info)))
					C.free(unsafe.Pointer(fn))
					if t.obj.handle == norm_object_handle(C.NORM_OBJECT_INVALID) {
						t.errc <- E_object_invalid
					} else {
						o.object_id_ct++
						t.obj.Id = o.object_id_ct
						t.objects[t.obj.handle] = t.obj
						t.errc <- nil
					}
				case icmd_send_command:
					if bool(C.NormSendCommand(t.handle, b2c(t.buf), b2uint(t.buf), C._Bool(t.enable))) {
						t.errc <- nil
					} else {
						t.errc <- E_false
					}
				case icmd_cancel_command:
					C.NormCancelCommand(t.handle)
					t.errc <- nil
				case icmd_get_tx_rate:
					t.double = float64(C.NormGetTxRate(t.handle))
					t.errc <- nil
				case icmd_set_tx_rate:
					C.NormSetTxRate(t.handle, C.double(t.double))
					t.errc <- nil
				case icmd_set_tx_rate_bounds:
					C.NormSetTxRateBounds(t.handle, C.double(t.double), C.double(t.double2))
					t.errc <- nil
				case icmd_set_tx_socket_buffer:
					if bool(C.NormSetTxSocketBuffer(t.handle, C.uint(t.size))) {
						t.errc <- nil
					} else {
						t.errc <- E_false
					}
				case icmd_set_rx_socket_buffer:
					if bool(C.NormSetRxSocketBuffer(t.handle, C.uint(t.size))) {
						t.errc <- nil
					} else {
						t.errc <- E_false
					}
				case icmd_get_local_node_id:
					t.node_id = norm_node_id(C.NormGetLocalNodeId(t.handle))
					t.errc <- nil
				case icmd_set_multicast_interface:
					ba := new_address(t.bind_address)
					if bool(C.NormSetMulticastInterface(t.handle, ba.p)) {
						ba.free()
						t.errc <- nil
					} else {
						ba.free()
						t.errc <- E_false
					}
				case icmd_set_ssm:
					ba := new_address(t.bind_address)
					if bool(C.NormSetSSM(t.handle, ba.p)) {
						ba.free()
						t.errc <- nil
					} else {
						ba.free()
						t.errc <- E_false
					}
				case icmd_set_ttl:
					if bool(C.NormSetTTL(t.handle, C.uchar(t.ttl_tos))) {
						t.errc <- nil
					} else {
						t.errc <- E_false
					}
				case icmd_set_tos:
					if bool(C.NormSetTOS(t.handle, C.uchar(t.ttl_tos))) {
						t.errc <- nil
					} else {
						t.errc <- E_false
					}
				case icmd_set_fragmentation:
					if bool(C.NormSetFragmentation(t.handle, C._Bool(t.enable))) {
						t.errc <- nil
					} else {
						t.errc <- E_false
					}
				case icmd_set_flow_control:
					C.NormSetFlowControl(t.handle, C.double(t.double))
					t.errc <- nil
				case icmd_get_grtt_estimate:
					t.double = float64(C.NormGetGrttEstimate(t.handle))
					t.errc <- nil
				case icmd_set_grtt_estimate:
					C.NormSetGrttEstimate(t.handle, C.double(t.double))
					t.errc <- nil
				case icmd_set_grtt_max:
					C.NormSetGrttMax(t.handle, C.double(t.double))
					t.errc <- nil
				case icmd_set_grtt_probing_mode:
					C.NormSetGrttProbingMode(t.handle, C.NormProbingMode(t.probe_mode))
					t.errc <- nil
				case icmd_set_grtt_probing_interval:
					C.NormSetGrttProbingInterval(t.handle, C.double(t.double), C.double(t.double2))
					t.errc <- nil
				case icmd_set_backoff_factor:
					C.NormSetBackoffFactor(t.handle, C.double(t.double))
					t.errc <- nil
				case icmd_set_group_size:
					C.NormSetGroupSize(t.handle, C.uint(t.size))
					t.errc <- nil
				case icmd_set_tx_robust_factor:
					C.NormSetTxRobustFactor(t.handle, C.int(t.i))
					t.errc <- nil
				case icmd_set_silent_receiver:
					C.NormSetSilentReceiver(t.handle, C._Bool(t.enable), C.int(t.i))
					t.errc <- nil
				case icmd_set_default_sync_policy:
					C.NormSetDefaultSyncPolicy(t.handle, C.NormSyncPolicy(t.sync_policy))
					t.errc <- nil
				case icmd_set_default_nacking_mode:
					C.NormSetDefaultNackingMode(t.handle, C.NormNackingMode(t.nacking_mode))
					t.errc <- nil
				case icmd_node_set_nacking_mode:
					C.NormNodeSetNackingMode(t.handle, C.NormNackingMode(t.nacking_mode))
					t.errc <- nil
				case icmd_set_default_repair_boundary:
					C.NormSetDefaultRepairBoundary(t.handle, C.NormRepairBoundary(t.repair_boundary))
					t.errc <- nil
				case icmd_set_default_rx_robust_factor:
					C.NormSetDefaultRxRobustFactor(t.handle, C.int(t.i))
					t.errc <- nil
				}
			case *Object:
				switch t.cmd {
				case icmd_stream_read:
					ret := false
					if t.stream_read_buf == nil {
						olen := C.NormObjectGetSize(t.handle)
						if C.NormSize(Max_int) < olen {
							goto end_read
						}
						t.stream_read_buf = bytes.NewBuffer(make([]byte, olen))
					}
					t.buf = &bytes.Buffer{}
					for num_bytes := C.uint(uint(t.stream_read_buf.Len())); 0 < num_bytes; {
						num_bytes = C.uint(uint(t.stream_read_buf.Len()))
						ret = bool(C.NormStreamRead(t.handle, b2c(t.stream_read_buf), &num_bytes))
						if 0 < num_bytes {
							if n, err := t.buf.Write(t.stream_read_buf.Bytes()[0:num_bytes]); err != nil || C.uint(uint(n)) != num_bytes {
								goto end_read
							}
						}
					}
				end_read:
					if ret {
						t.errc <- nil
					} else {
						t.errc <- E_false
					}
				case icmd_stream_flush:
					C.NormStreamFlush(t.handle, C._Bool(t.enable), C.NormFlushMode(t.flush_mode))
					t.errc <- nil
				case icmd_stream_set_auto_flush:
					C.NormStreamSetAutoFlush(t.handle, C.NormFlushMode(t.flush_mode))
					t.errc <- nil
				case icmd_stream_set_push_enable:
					C.NormStreamSetPushEnable(t.handle, C._Bool(t.enable))
					t.errc <- nil
				case icmd_stream_mark_eom:
					C.NormStreamMarkEom(t.handle)
					t.errc <- nil
				case icmd_stream_has_vacancy:
					if bool(C.NormStreamHasVacancy(t.handle)) {
						t.errc <- nil
					} else {
						t.errc <- E_false
					}
				case icmd_stream_write:
					t.written = int(C.NormStreamWrite(t.handle, b2c(t.buf), b2uint(t.buf)))
					t.errc <- nil
				case icmd_stream_seek_msg_start:
					if C.NormStreamSeekMsgStart(t.handle) {
						t.errc <- nil
					} else {
						t.errc <- E_false
					}
				case icmd_stream_get_read_offset:
					t.offset = uint64(C.NormStreamGetReadOffset(t.handle))
					t.errc <- nil
				case icmd_object_cancel:
					C.NormObjectCancel(t.handle)
					t.errc <- nil
				case icmd_object_get_info:
					if olen := C.NormObjectGetInfoLength(t.handle); 0 < olen {
						t.buf = bytes.NewBuffer(make([]byte, olen))
						if mlen := C.NormObjectGetInfo(t.handle, b2c(t.buf), b2UINT16(t.buf)); mlen != olen {
							t.buf.Reset()
						}
					} else {
						t.buf = &bytes.Buffer{}
					}
					t.errc <- nil
				case icmd_object_get_size:
					t.size = uint64(C.NormObjectGetSize(t.handle))
					t.errc <- nil
				case icmd_object_get_bytes_pending:
					t.size = uint64(C.NormObjectGetBytesPending(t.handle))
					t.errc <- nil
				case icmd_object_retain:
					o.object_retain(t)
					t.errc <- nil
				case icmd_object_release:
					o.object_release(t)
					t.errc <- nil
				case icmd_file_get_name:
					t.buf = bytes.NewBuffer(make([]byte, syscall.PathMax+1))
					if !C.NormFileGetName(t.handle, b2c(t.buf), C.uint(t.buf.Len())) {
						t.buf.Reset()
					}
					t.errc <- nil
				case icmd_file_rename:
					fn := C.CString(t.file_name)
					ret := bool(C.NormFileRename(t.handle, fn))
					C.free(unsafe.Pointer(fn))
					if ret {
						t.errc <- nil
					} else {
						t.errc <- E_false
					}
				case icmd_data_access_data:
					if olen := C.NormObjectGetSize(t.handle); 0 < olen {
						if p := C.NormDataAccessData(t.handle); p == nil {
							t.buf = &bytes.Buffer{}
						} else {
							t.buf = bytes.NewBuffer(C.GoBytes(unsafe.Pointer(C.NormDataAccessData(t.handle)), C.int(olen)))
						}
					} else {
						t.buf = &bytes.Buffer{}
					}
					if t.data_access_data_release {
						o.object_release(t)
					}
					t.errc <- nil
				case icmd_object_get_sender:
					if h := C.NormObjectGetSender(t.handle); h != C.NORM_NODE_INVALID {
						t.node = new_node(t.icmdc, norm_node_handle(h))
					}
					t.errc <- nil
				case icmd_stream_close:
					C.NormStreamClose(t.handle, C._Bool(t.enable))
					t.errc <- nil
				case icmd_object_has_info:
					if bool(C.NormObjectHasInfo(t.handle)) {
						t.errc <- nil
					} else {
						t.errc <- E_false
					}
				case icmd_object_get_info_length:
					t.i = int(C.NormObjectGetInfoLength(t.handle))
					t.errc <- nil
				}
			case *Node:
				switch t.cmd {
				case icmd_node_get_id:
					t.id = uint32(C.NormNodeGetId(t.handle))
					t.errc <- nil
				case icmd_node_get_address:
					addr_len := C.uint(46)
					addr := make([]byte, int(addr_len))
					port := C.UINT16(0)
					if C.NormNodeGetAddress(t.handle, (*C.char)(unsafe.Pointer(&addr[0])), &addr_len, &port) {
						t.address = net.IP((addr[0:addr_len])).String()
						t.port = uint16(port)
						t.errc <- nil
					} else {
						t.errc <- E_false
					}
				case icmd_node_get_command:
					var buflen C.uint
					t.command = &bytes.Buffer{}
					C.NormNodeGetCommand(t.handle, nil, &buflen)
					if 0 < buflen {
						t.command = bytes.NewBuffer(make([]byte, buflen))
						C.NormNodeGetCommand(t.handle, b2c(t.command), &buflen)
					}
					t.errc <- nil
				case icmd_node_set_unicast_nack:
					C.NormNodeSetUnicastNack(t.handle, C._Bool(t.enable))
					t.errc <- nil
				case icmd_object_set_nacking_mode:
					C.NormObjectSetNackingMode(t.handle, C.NormNackingMode(t.nacking_mode))
					t.errc <- nil
				case icmd_node_set_repair_boundary:
					C.NormNodeSetRepairBoundary(t.handle, C.NormRepairBoundary(t.repair_boundary))
					t.errc <- nil
				case icmd_node_get_grtt:
					t.f64 = float64(C.NormNodeGetGrtt(t.handle))
					t.errc <- nil
				case icmd_node_free_buffers:
					C.NormNodeFreeBuffers(t.handle)
					t.errc <- nil
				case icmd_node_delete:
					C.NormNodeDelete(t.handle)
					t.errc <- nil
				case icmd_node_retain:
					C.NormNodeRetain(t.handle)
					t.errc <- nil
				case icmd_node_release:
					C.NormNodeRelease(t.handle)
					t.errc <- nil
				}
			}
		case _, ok := <-desc_ready:
			if !ok {
				return
			}
			if err = o.do_event(); err != nil {
				return
			}
			desc_ready_received <- struct{}{}
		}
	}
}

func (o *Instance) do_event() error {
	if !C.NormGetNextEvent(o.handle, &o.nevent, true) {
		return nil
	}
	ev := Event_type(1) << Event_type(o.nevent._type)
	// Allow updated, completed or check subscribed Events
	if ev&o.sess().events == 0 {
		return nil
	}
	switch ev {
	case Event_type_tx_object_purged:
		o.sess().c <- new_event(ev, o.obj())
		o.sess().release(norm_object_handle(o.nevent.object))
	case Event_type_tx_object_sent, Event_type_rx_object_info, Event_type_rx_object_updated:
		o.sess().c <- new_event(ev, o.obj())
	case Event_type_remote_sender_new, Event_type_remote_sender_active, Event_type_remote_sender_inactive,
		Event_type_remote_sender_purged, Event_type_rx_cmd_new:
		e := new_event(ev, nil)
		e.Node = new_node(o.icmdc, norm_node_handle(o.nevent.sender))
		o.sess().c <- e
	case Event_type_rx_object_new:
		obj := new_object_buf(o.sess().icmdc, nil, nil, Object_type(C.NormObjectGetType(o.nevent.object)))
		obj.handle = norm_object_handle(o.nevent.object)
		o.object_id_ct++
		obj.Id = o.object_id_ct
		o.sess().objects[obj.handle] = obj
		o.sess().c <- new_event(ev, obj)
	case Event_type_rx_object_aborted:
		if o.obj() != nil {
			o.sess().release(o.obj().handle)
		}
		o.sess().c <- new_event(ev, o.obj())
	case Event_type_rx_object_completed:
		obj := o.obj()
		switch obj.ot {
		case Object_type_stream:
		case Object_type_file, Object_type_data:
			o.object_retain(obj)
			o.sess().release(obj.handle)
		}
		o.sess().c <- new_event(ev, obj)
	case Event_type_tx_queue_empty, Event_type_tx_queue_vacancy, Event_type_cc_active,
		Event_type_cc_inactive, Event_type_send_error, Event_type_tx_flush_completed,
		Event_type_tx_watermark_completed:
		o.sess().c <- new_event(ev, nil)
	case Event_type_grtt_updated:
		e := new_event(ev, nil)
		e.Grtt = float64(C.NormGetGrttEstimate(o.nevent.session))
		o.sess().c <- e
	case Event_type_tx_rate_changed:
		e := new_event(ev, nil)
		e.Tx_rate = float64(C.NormGetTxRate(o.nevent.session))
		o.sess().c <- e
	case Event_type_tx_cmd_sent:
		o.sess().c <- new_event(ev, nil)
	case Event_type_invalid:
		return fmt.Errorf("invalid event")
	}
	return nil
}

func (o *Instance) descriptor_ready(desc *os.File, desc_ready chan<- struct{}, desc_ready_received <-chan struct{}, shutdown *os.File) {
	defer o.wg.Done()
	epfd, err := syscall.EpollCreate(2)
	if err != nil {
		return
	}
	err = syscall.EpollCtl(epfd, syscall.EPOLL_CTL_ADD, int(desc.Fd()), &syscall.EpollEvent{Events: syscall.EPOLLIN | syscall.EPOLLHUP, Fd: int32(desc.Fd())})
	if err != nil {
		return
	}
	defer syscall.Close(epfd)
	defer syscall.EpollCtl(epfd, syscall.EPOLL_CTL_DEL, int(desc.Fd()), &syscall.EpollEvent{})
	err = syscall.EpollCtl(epfd, syscall.EPOLL_CTL_ADD, int(shutdown.Fd()), &syscall.EpollEvent{Events: syscall.EPOLLIN | syscall.EPOLLHUP, Fd: int32(shutdown.Fd())})
	if err != nil {
		return
	}
	defer shutdown.Close()
	defer syscall.EpollCtl(epfd, syscall.EPOLL_CTL_DEL, int(shutdown.Fd()), &syscall.EpollEvent{})
	events := make([]syscall.EpollEvent, 2)
	var ok bool
	for {
		_, ok = <-desc_ready_received
		if !ok {
			return
		}
		n, err := syscall.EpollWait(epfd, events, -1)
		if err != nil {
			return
		}
		for i := 0; i < n; i++ {
			switch {
			case events[i].Fd == int32(desc.Fd()):
				desc_ready <- struct{}{}
			case events[i].Fd == int32(shutdown.Fd()):
				return
			}
		}
	}
}

func (o *Instance) object_retain(obj *Object) {
	obj.retained = true
	C.NormObjectRetain(obj.handle)
}

func (o *Instance) object_release(obj *Object) {
	obj.retained = false
	C.NormObjectRelease(obj.handle)
}

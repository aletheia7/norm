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
	// "log"
	"net"
	"os"
	"sync"
	"syscall"
	"time"
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

func create_instance(wg *sync.WaitGroup) (r *Instance, err error) {
	r = &Instance{
		wg:      wg,
		sess_db: refdb{},
	}
	r.ctx, r.cancel = context.WithCancel(context.Background())
	if r.handle = norm_instance_handle(C.NormCreateInstance(false)); r.handle == norm_instance_handle(C.NORM_INSTANCE_INVALID) {
		return nil, errors.New("instance invalid")
	}
	wg.Add(1)
	go r.loop()
	return
}

func (o *Instance) stop() {
	C.NormStopInstance(o.handle)
}

func (o *Instance) restart() bool {
	return bool(C.NormRestartInstance(o.handle))
}

func (o *Instance) set_cache_directory(cache_path string) bool {
	cp := C.CString(cache_path)
	defer C.free(unsafe.Pointer(cp))
	return bool(C.NormSetCacheDirectory(o.handle, cp))
}

func (o *Instance) set_debug_level(debug_level uint) {
	C.NormSetDebugLevel(C.uint(debug_level))
}

func (o *Instance) open_debug_log(file_name string) bool {
	fn := C.CString(file_name)
	defer C.free(unsafe.Pointer(fn))
	return bool(C.NormOpenDebugLog(o.handle, fn))
}

func (o *Instance) close_debug_log() {
	C.NormCloseDebugLog(o.handle)
}

func (o *Instance) get_debug_level() uint {
	return uint(C.NormGetDebugLevel())
}

func (o *Instance) open_debug_pipe(file_name string) bool {
	fn := C.CString(file_name)
	defer C.free(unsafe.Pointer(fn))
	return bool(C.NormOpenDebugPipe(o.handle, fn))
}

func (o *Instance) close_debug_pipe() {
	C.NormCloseDebugPipe(o.handle)
}

func (o *Instance) get_version() string {
	var major, minor, patch C.int
	C.NormGetVersion(&major, &minor, &patch)
	return fmt.Sprintf("%v.%v.%v", major, minor, patch)
}

func (o *Instance) create_session(address string, port int, node_id Node_id) (sess *Session, err error) {
	sess = &Session{
		i:       o,
		c:       make(chan *Event, 100),
		events:  Event_type_all,
		objects: map[norm_object_handle]*Object{},
	}
	node_id_ := norm_node_id(node_id)
	if node_id_ == 0 {
		node_id_ = norm_node_id(uint32(time.Now().Unix()))
	}
	ba := new_address(address)
	defer ba.free()
	sess.handle = norm_session_handle(C.NormCreateSession(o.handle, ba.p, C.UINT16(port), C.NormNodeId(node_id)))
	if sess.handle == norm_session_handle(C.NORM_SESSION_INVALID) {
		sess = nil
		err = E_session_invalid
	} else {
		o.sess_db.add(sess)
	}
	return
}

func (o *Session) destroy() {
	C.NormDestroySession(o.handle)
	o.i.sess_db.remove(o)
}

func (o *Session) set_message_trace(trace bool) {
	C.NormSetMessageTrace(o.handle, C._Bool(trace))
}

func (o *Session) get_tx_rate() float64 {
	return float64(C.NormGetTxRate(o.handle))
}

func (o *Session) set_tx_rate(tx_rate float64) {
	C.NormSetTxRate(o.handle, C.double(tx_rate))
}

func (o *Session) set_tx_rate_bounds(min, max float64) {
	C.NormSetTxRateBounds(o.handle, C.double(min), C.double(max))
}

func (o *Session) start_sender(id norm_session_id, buffer_space uint, segment_size, block_size, num_parity uint16, fec_id uint8) bool {
	return bool(C.NormStartSender(o.handle, C.NormSessionId(id), C.UINT32(buffer_space), C.UINT16(segment_size), C.UINT16(block_size), C.UINT16(num_parity), C.UINT8(fec_id)))
}
func (o *Session) set_congestion_control(enable, adjust_rate bool) {
	C.NormSetCongestionControl(o.handle, C._Bool(enable), C._Bool(adjust_rate))
}

func (o *Session) stop_sender() {
	C.NormStopSender(o.handle)
}

func (o *Session) set_tx_only(tx_only, connect_to_session_address bool) {
	C.NormSetTxOnly(o.handle, C._Bool(tx_only), C._Bool(connect_to_session_address))
}

func (o *Session) data_enqueue(data, info []byte) (*Object, error) {
	obj := new_object(data, info, Object_type_data)
	if obj.handle = norm_object_handle(C.NormDataEnqueue(o.handle, b2c(obj.data), b2UINT32(obj.data), b2c(obj.info), b2uint(obj.info))); obj.handle == norm_object_handle(C.NORM_OBJECT_INVALID) {
		return nil, E_object_invalid
	}
	object_id_ct++
	obj.Id = object_id_ct
	o.objects[obj.handle] = obj
	return obj, nil
}

func (o *Session) requeue_object(obj *Object) bool {
	if C.NormRequeueObject(o.handle, obj.handle) {
		return true
	}
	return false
}

func (o *Session) file_enqueue(file_name string, info []byte) (*Object, error) {
	obj := new_object(nil, info, Object_type_file)
	fn := C.CString(file_name)
	defer C.free(unsafe.Pointer(fn))
	obj.handle = norm_object_handle(C.NormFileEnqueue(o.handle, fn, b2c(obj.info), b2uint(obj.info)))
	if obj.handle == norm_object_handle(C.NORM_OBJECT_INVALID) {
		return nil, E_object_invalid
	}
	object_id_ct++
	obj.Id = object_id_ct
	o.objects[obj.handle] = obj
	return obj, nil
}

func (o *Session) set_loopback(enable bool) {
	C.NormSetLoopback(o.handle, C._Bool(enable))
}

func (o *Session) set_tx_port(tx_port uint16, tx_port_reuse bool, tx_bind_address string) bool {
	ba := new_address(tx_bind_address)
	defer ba.free()
	return bool(C.NormSetTxPort(o.handle, C.UINT16(tx_port), C._Bool(tx_port_reuse), ba.p))
}

func (o *Session) set_tx_cache_bounds(size_max uint, count_min, count_max uint32) {
	C.NormSetTxCacheBounds(o.handle, C.NormSize(size_max), C.UINT32(count_min), C.UINT32(count_max))
}

func (o *Session) add_acking_node(node_id Node_id) bool {
	return bool(C.NormAddAckingNode(o.handle, C.NormNodeId(node_id)))
}

func (o *Session) add_remove_acking_node(node_id uint32) {
	C.NormRemoveAckingNode(o.handle, C.NormNodeId(node_id))
}

func (o *Session) get_next_acking_node(status Acking_status) (node_id uint32, success bool) {
	// 	// todo_brian: sig diff
	success = bool(C.NormGetNextAckingNode(o.handle, (*C.NormNodeId)(&node_id), (*C.NormAckingStatus)(&status)))
	return
}

func (o *Session) get_acking_status(node_id uint32) Acking_status {
	return Acking_status(C.NormGetAckingStatus(o.handle, C.NormNodeId(node_id)))
}

func (o *Session) set_watermark(obj *Object, override_flush bool) bool {
	return bool(C.NormSetWatermark(o.handle, obj.handle, C._Bool(override_flush)))
}

func (o *Session) cancel_watermark() {
	// todo brian sig
	C.NormCancelWatermark(o.handle)
}

func (o *Session) set_default_unicast_nack(enable bool) {
	C.NormSetDefaultUnicastNack(o.handle, C._Bool(enable))
}

func (o *Session) set_default_repair_boundary(boundary Repair_boundary) {
	C.NormSetDefaultRepairBoundary(o.handle, C.NormRepairBoundary(boundary))
}

func (o *Session) set_rx_port_reuse(rx_port_reuse bool, rx_bind_address, rx_sender_address string, rx_sender_address_port uint16) {
	ba := new_address(rx_bind_address)
	ba.free()
	sa := new_address(rx_sender_address)
	sa.free()
	C.NormSetRxPortReuse(o.handle, C._Bool(rx_port_reuse), ba.p, sa.p, (C.UINT16)(rx_sender_address_port))
}

func (o *Session) start_receiver(buffer uint) bool {
	return bool(C.NormStartReceiver(o.handle, C.UINT32(buffer)))
}

func (o *Session) stop_receiver() {
	C.NormStopReceiver(o.handle)
	o.release_all()
}

func (o *Session) set_silent_receiver(silent bool, max_delay int) {
	C.NormSetSilentReceiver(o.handle, C._Bool(silent), C.int(max_delay))
}

func (o *Session) set_rx_cache_limit(rx_cache_limit int) {
	C.NormSetRxCacheLimit(o.handle, C.ushort(rx_cache_limit))
}

func (o *Session) set_default_sync_Policy(policy Sync_policy) {
	C.NormSetDefaultSyncPolicy(o.handle, C.NormSyncPolicy(policy))
}

func (o *Session) set_default_rx_robust_factor(rx_robust_factor int) {
	C.NormSetDefaultRxRobustFactor(o.handle, C.int(rx_robust_factor))
}

func (o *Session) stream_open(buffer_size int, info []byte) (obj *Object, err error) {
	obj = new_object(nil, info, Object_type_stream)
	if obj.handle = norm_object_handle(C.NormStreamOpen(o.handle, C.UINT32(buffer_size), b2c(obj.info), b2uint(obj.info))); obj.handle == norm_object_handle(C.NORM_OBJECT_INVALID) {
		obj = nil
		err = E_object_invalid
	} else {
		object_id_ct++
		obj.Id = object_id_ct
		o.objects[obj.handle] = obj
	}
	return
}

func (o *Session) set_default_nacking_mode(mode Nacking_mode) {
	C.NormSetDefaultNackingMode(o.handle, C.NormNackingMode(mode))
}

func (o *Session) send_command(cmd []byte, robust bool) bool {
	buf := bytes.NewBuffer(cmd)
	return bool(C.NormSendCommand(o.handle, b2c(buf), b2uint(buf), C._Bool(robust)))
}

func (o *Session) cancel_command() {
	C.NormCancelCommand(o.handle)
}

func (o *Session) set_tx_socket_buffer(buffer_size uint) bool {
	return bool(C.NormSetTxSocketBuffer(o.handle, C.uint(buffer_size)))
}

func (o *Session) set_flow_control(factor float64) {
	C.NormSetFlowControl(o.handle, C.double(factor))
}

func (o *Session) set_auto_parity(auto_parity uint8) {
	C.NormSetAutoParity(o.handle, C.uchar(auto_parity))
}

func (o *Session) set_rx_socket_buffer(buffer_size uint) bool {
	return bool(C.NormSetRxSocketBuffer(o.handle, C.uint(buffer_size)))
}

func (o *Session) get_local_node_id() Node_id {
	return Node_id(C.NormGetLocalNodeId(o.handle))
}

func (o *Session) set_multicast_interface(address string) bool {
	ba := new_address(address)
	defer ba.free()
	return bool(C.NormSetMulticastInterface(o.handle, ba.p))
}

func (o *Session) set_ssm(address string) bool {
	ba := new_address(address)
	defer ba.free()
	return bool(C.NormSetSSM(o.handle, ba.p))
}

func (o *Session) set_ttl(ttl uint8) bool {
	return bool(C.NormSetTTL(o.handle, C.uchar(ttl)))
}

func (o *Session) set_tos(tos uint8) bool {
	return bool(C.NormSetTOS(o.handle, C.uchar(tos)))
}

func (o *Session) set_fragmentation(enable bool) bool {
	return bool(C.NormSetFragmentation(o.handle, C._Bool(enable)))
}

func (o *Session) get_grtt_estimate() float64 {
	return float64(C.NormGetGrttEstimate(o.handle))
}

func (o *Session) set_grtt_estimate(grtt float64) {
	C.NormSetGrttEstimate(o.handle, C.double(grtt))
}

func (o *Session) set_grtt_max(max float64) {
	C.NormSetGrttMax(o.handle, C.double(max))
}

func (o *Session) set_grtt_probing_mode(mode Probe_mode) {
	C.NormSetGrttProbingMode(o.handle, C.NormProbingMode(mode))
}

func (o *Session) set_grtt_probing_interval(min, max float64) {
	C.NormSetGrttProbingInterval(o.handle, C.double(min), C.double(max))
}

func (o *Session) set_backoff_factor(factor float64) {
	C.NormSetBackoffFactor(o.handle, C.double(factor))
}

func (o *Session) set_group_size(size uint) {
	C.NormSetGroupSize(o.handle, C.uint(size))
}

func (o *Session) set_tx_robust_factor(factor int) {
	C.NormSetTxRobustFactor(o.handle, C.int(factor))
}

func (o *Object) file_rename(new_file_name string) bool {
	fn := C.CString(new_file_name)
	defer C.free(unsafe.Pointer(fn))
	return bool(C.NormFileRename(o.handle, fn))
}

func (o *Object) has_info() bool {
	return bool(C.NormObjectHasInfo(o.handle))
}

func (o *Object) get_info_length() int {
	return int(C.NormObjectGetInfoLength(o.handle))
}

func (o *Object) get_info() (info *bytes.Buffer) {
	if olen := C.NormObjectGetInfoLength(o.handle); 0 < olen {
		info = bytes.NewBuffer(make([]byte, olen))
		if mlen := C.NormObjectGetInfo(o.handle, b2c(info), b2UINT16(info)); mlen != olen {
			info.Reset()
		}
	} else {
		info = &bytes.Buffer{}
	}
	return
}

func (o *Object) get_size() uint64 {
	return uint64(C.NormObjectGetSize(o.handle))
}

func (o *Object) get_bytes_pending() uint64 {
	return uint64(C.NormObjectGetBytesPending(o.handle))
}

func (o *Object) cancel() {
	C.NormObjectCancel(o.handle)
}

func (o *Object) retain() {
	o.retained = true
	C.NormObjectRetain(o.handle)
}

func (o *Object) release() {
	o.retained = false
	C.NormObjectRelease(o.handle)
}

func (o *Object) stream_read() (data *bytes.Buffer, ret bool) {
	data = &bytes.Buffer{}
	if o.stream_read_buf == nil {
		olen := C.NormObjectGetSize(o.handle)
		if C.NormSize(Max_int) < olen {
			return
		}
		o.stream_read_buf = bytes.NewBuffer(make([]byte, olen))
	}
	for num_bytes := C.uint(uint(o.stream_read_buf.Len())); 0 < num_bytes; {
		num_bytes = C.uint(uint(o.stream_read_buf.Len()))
		ret = bool(C.NormStreamRead(o.handle, b2c(o.stream_read_buf), &num_bytes))
		if 0 < num_bytes {
			if n, err := data.Write(o.stream_read_buf.Bytes()[:num_bytes]); err != nil || C.uint(uint(n)) != num_bytes {
				return
			}
		}
	}
	return
}

func (o *Object) stream_write(data []byte) int {
	buf := bytes.NewBuffer(data)
	return int(C.NormStreamWrite(o.handle, b2c(buf), b2uint(buf)))
}

func (o *Object) stream_close(graceful bool) {
	C.NormStreamClose(o.handle, C._Bool(graceful))
}

func (o *Object) stream_mark_eom() {
	C.NormStreamMarkEom(o.handle)
}

func (o *Object) stream_flush(eom bool, mode Flush_mode) {
	C.NormStreamFlush(o.handle, C._Bool(eom), C.NormFlushMode(mode))
}

func (o *Object) stream_set_auto_flush(mode Flush_mode) {
	C.NormStreamSetAutoFlush(o.handle, C.NormFlushMode(mode))
}

func (o *Object) stream_set_push_enable(enable bool) {
	C.NormStreamSetPushEnable(o.handle, C._Bool(enable))
}

func (o *Object) stream_has_vacancy() bool {
	return bool(C.NormStreamHasVacancy(o.handle))
}

func (o *Object) stream_seek_msg_start() bool {
	return bool(C.NormStreamSeekMsgStart(o.handle))
}

func (o *Object) stream_get_read_offset() uint64 {
	return uint64(C.NormStreamGetReadOffset(o.handle))
}

func (o *Object) set_nacking_mode(mode Nacking_mode) {
	C.NormObjectSetNackingMode(o.handle, C.NormNackingMode(mode))
}

func (o *Object) file_get_name() (file_name *bytes.Buffer) {
	file_name = bytes.NewBuffer(make([]byte, syscall.PathMax+1))
	if !C.NormFileGetName(o.handle, b2c(file_name), C.uint(file_name.Len())) {
		file_name.Reset()
	}
	return
}

func (o *Object) data_access_data(release bool) (data *bytes.Buffer) {
	if olen := C.NormObjectGetSize(o.handle); 0 < olen {
		if p := C.NormDataAccessData(o.handle); p == nil {
			data = &bytes.Buffer{}
		} else {
			data = bytes.NewBuffer(C.GoBytes(unsafe.Pointer(C.NormDataAccessData(o.handle)), C.int(olen)))
		}
	} else {
		data = &bytes.Buffer{}
	}
	if release {
		o.release()
	}
	return
}

func (o *Object) get_sender() (node *Node) {
	if h := C.NormObjectGetSender(o.handle); h != C.NORM_NODE_INVALID {
		node = new_node(norm_node_handle(h))
	}
	return
}

func (o *Node) get_id() uint32 {
	return uint32(C.NormNodeGetId(o.handle))
}

func (o *Node) get_address() (address string, port uint16, success bool) {
	addr_len := C.uint(46)
	addr := make([]byte, int(addr_len))
	p := C.UINT16(0)
	success = bool(C.NormNodeGetAddress(o.handle, (*C.char)(unsafe.Pointer(&addr[0])), &addr_len, &p))
	if success {
		address = net.IP((addr[0:addr_len])).String()
		port = uint16(p)
	}
	return
}

func (o *Node) get_command() (cmd *bytes.Buffer) {
	var buflen C.uint
	cmd = &bytes.Buffer{}
	C.NormNodeGetCommand(o.handle, nil, &buflen)
	if 0 < buflen {
		cmd = bytes.NewBuffer(make([]byte, buflen))
		C.NormNodeGetCommand(o.handle, b2c(cmd), &buflen)
	}
	return
}

func (o *Node) set_unicast_nack(enable bool) {
	C.NormNodeSetUnicastNack(o.handle, C._Bool(enable))
}

func (o *Node) set_nacking_mode(mode Nacking_mode) {
	C.NormNodeSetNackingMode(o.handle, C.NormNackingMode(mode))
}

func (o *Node) set_repair_boundary(boundary Repair_boundary) {
	C.NormNodeSetRepairBoundary(o.handle, C.NormRepairBoundary(boundary))
}

func (o *Node) get_grtt() float64 {
	return float64(C.NormNodeGetGrtt(o.handle))
}

func (o *Node) free_buffers() {
	C.NormNodeFreeBuffers(o.handle)
}

func (o *Node) delete() {
	C.NormNodeDelete(o.handle)
}

func (o *Node) retain() {
	C.NormNodeRetain(o.handle)
}

func (o *Node) release() {
	C.NormNodeRelease(o.handle)
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

func (o *Instance) loop() {
	defer o.wg.Done()
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
	o.wg.Add(1)
	go o.descriptor_ready(shutdown_r)
	<-o.ctx.Done()
	shutdown_w.Close()
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
		e.Node = new_node(norm_node_handle(o.nevent.sender))
		o.sess().c <- e
	case Event_type_rx_object_new:
		obj := new_object_buf(nil, nil, Object_type(C.NormObjectGetType(o.nevent.object)))
		obj.handle = norm_object_handle(o.nevent.object)
		object_id_ct++
		obj.Id = object_id_ct
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
			obj.retain()
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

func (o *Instance) descriptor_ready(shutdown *os.File) {
	defer o.wg.Done()
	var desc *os.File
	norm_descriptor := C.NormGetDescriptor(o.handle)
	if norm_descriptor == C.NORM_DESCRIPTOR_INVALID {
		return
	}
	desc = os.NewFile(uintptr(norm_descriptor), "norm descriptor")
	defer func() {
		syscall.Close(int(desc.Fd()))
		C.NormDestroyInstance(o.handle)
	}()
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
	for {
		select {
		case <-o.ctx.Done():
			return
		default:
			n, err := syscall.EpollWait(epfd, events, -1)
			if err != nil {
				return
			}
			for i := 0; i < n; i++ {
				switch {
				case events[i].Fd == int32(desc.Fd()):
					lock.Lock()
					o.do_event()
					lock.Unlock()
				case events[i].Fd == int32(shutdown.Fd()):
					return
				}
			}
		}
	}
}

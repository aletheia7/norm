// Copyright 2016 aletheia7. All rights reserved. Use of this source code is
// governed by a BSD-2-Clause license that can be found in the LICENSE file.

//go:generate go build go-norm-build/norm-build.go
//go:generate norm-build
//go:generate rm norm-build

package norm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	Max_rx_cache_limit = 16384
	Max_int            = int(^uint(0) >> 1)
)

var (
	E_session_invalid               = errors.New("session invalid")
	E_object_invalid                = errors.New("object invalid")
	E_false                         = errors.New("false")
	E_sender_sessions_not_destroyed error
	lock                            sync.Mutex
	object_id_ct                    uint64
)

type Event struct {
	Type    Event_type
	O       *Object
	Node    *Node
	Grtt    float64
	Tx_rate float64
}

func new_event(et Event_type, o *Object) *Event {
	return &Event{Type: et, O: o}
}

func (o Event) String() string {
	return fmt.Sprintf("%v, %v", o.Type, o.O)
}

type Event_type uint

// NormDeveloperGuide.html events:
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#d0e1384
//
const (
	// These events are bit flags and can be or'd together.
	// See Session.Set_events().
	Event_type_all                    = event_type_all
	Event_type_invalid                = event_type_invalid
	Event_type_tx_queue_vacancy       = event_type_tx_queue_vacancy
	Event_type_tx_queue_empty         = event_type_tx_queue_empty
	Event_type_tx_flush_completed     = event_type_tx_flush_completed
	Event_type_tx_watermark_completed = event_type_tx_watermark_completed
	Event_type_tx_cmd_sent            = event_type_tx_cmd_sent
	Event_type_tx_object_sent         = event_type_tx_object_sent
	Event_type_tx_object_purged       = event_type_tx_object_purged
	Event_type_tx_rate_changed        = event_type_tx_rate_changed
	Event_type_local_sender_closed    = event_type_local_sender_closed
	Event_type_remote_sender_new      = event_type_remote_sender_new
	Event_type_remote_sender_reset    = event_type_remote_sender_reset
	Event_type_remote_sender_address  = event_type_remote_sender_address
	Event_type_remote_sender_active   = event_type_remote_sender_active
	Event_type_remote_sender_inactive = event_type_remote_sender_inactive
	Event_type_remote_sender_purged   = event_type_remote_sender_purged
	Event_type_rx_cmd_new             = event_type_rx_cmd_new
	Event_type_rx_object_new          = event_type_rx_object_new
	Event_type_rx_object_info         = event_type_rx_object_info
	Event_type_rx_object_updated      = event_type_rx_object_updated

	// Not sent for Object_type_stream until Stream_close()
	// Object_type_data and Object_type_file are retained (Object.Retain())
	// automatically. Must call Object.Release() to release memory.
	Event_type_rx_object_completed = event_type_rx_object_completed

	Event_type_rx_object_aborted = event_type_rx_object_aborted
	Event_type_grtt_updated      = event_type_grtt_updated
	Event_type_cc_active         = event_type_cc_active
	Event_type_cc_inactive       = event_type_cc_inactive
	Event_type_acking_node_new   = event_type_acking_node_new
	Event_type_send_error        = event_type_send_error // ICMP error (e.g. destination unreachable)
	Event_type_user_timeout      = event_type_user_timeout
)

var et = map[Event_type]string{
	Event_type_all:                    "all",
	Event_type_invalid:                "event invalid",
	Event_type_tx_queue_vacancy:       "tx queue vacancy",
	Event_type_tx_queue_empty:         "tx queue empty",
	Event_type_tx_flush_completed:     "tx flush completed",
	Event_type_tx_watermark_completed: "tx watermark completed",
	Event_type_tx_cmd_sent:            "tx cmd sent",
	Event_type_tx_object_sent:         "tx object sent",
	Event_type_tx_object_purged:       "tx object purged",
	Event_type_tx_rate_changed:        "tx rate changed",
	Event_type_local_sender_closed:    "event_local_sender_closed",
	Event_type_remote_sender_new:      "remote sender new",
	Event_type_remote_sender_reset:    "remote sender reset",
	Event_type_remote_sender_address:  "remote sender address",
	Event_type_remote_sender_active:   "remote sender active",
	Event_type_remote_sender_inactive: "remote sender inactive",
	Event_type_remote_sender_purged:   "remote sender purged",
	Event_type_rx_cmd_new:             "rx cmd new",
	Event_type_rx_object_new:          "rx object new",
	Event_type_rx_object_info:         "rx object info",
	Event_type_rx_object_updated:      "rx object updated",
	Event_type_rx_object_completed:    "rx object completed",
	Event_type_rx_object_aborted:      "rx object aborted",
	Event_type_grtt_updated:           "grtt updated",
	Event_type_cc_active:              "cc active",
	Event_type_cc_inactive:            "cc inactive",
	Event_type_acking_node_new:        "acking node new",
	Event_type_send_error:             "send error",
	Event_type_user_timeout:           "user timeout",
}

func (o Event_type) String() string {
	if v, ok := et[o]; ok {
		return v
	}
	return ""
}

type Object_type uint32

func (o Object_type) String() string {
	if v, ok := ot[o]; ok {
		return v
	}
	panic(fmt.Sprintf("missing Object_type: %v", string(o)))
}

var ot = map[Object_type]string{
	Object_type_none:   "none object",
	Object_type_data:   "data object",
	Object_type_file:   "file object",
	Object_type_stream: "stream object",
}

const (
	Object_type_none   Object_type = object_type_none
	Object_type_data               = object_type_data
	Object_type_file               = object_type_file
	Object_type_stream             = object_type_stream
)

type Node_id norm_node_id

var nn = map[Node_id]string{
	Node_none: "node none",
	Node_any:  "node any",
}

func (o Node_id) String() string {
	if s, ok := nn[o]; ok {
		return s
	}
	return fmt.Sprintf("node %x", string(o))
}

var (
	Node_none Node_id = node_none
	Node_any          = node_any
)

var as = map[Acking_status]string{
	Ack_invalid: "ack invalid",
	Ack_failure: "ack failure",
	Ack_pending: "ack pending",
	Ack_success: "ack success",
}

type Acking_status norm_acking_status

const (
	Ack_invalid Acking_status = ack_invalid
	Ack_failure               = ack_failure
	Ack_pending               = ack_pending
	Ack_success               = ack_success
)

func (o Acking_status) String() string {
	return as[o]
}

type Sync_policy norm_sync_policy

const (
	Sync_current Sync_policy = sync_current
	Sync_all                 = sync_all
)

var sp = map[Sync_policy]string{
	Sync_current: "sync current",
	Sync_all:     "sync all",
}

func (o Sync_policy) String() string {
	return sp[o]
}

type Nacking_mode norm_nacking_mode

const (
	Nack_none      Nacking_mode = nack_none
	Nack_info_only              = nack_info_only
	Nack_normal                 = nack_normal
)

var nm = map[Nacking_mode]string{
	Nack_none:      "nack none",
	Nack_info_only: "nack info only",
	Nack_normal:    "nack normal",
}

func (o Nacking_mode) String() string {
	return nm[o]
}

type Repair_boundary norm_repair_boundary

var rb = map[Repair_boundary]string{
	Boundary_block:  "repair block",
	Boundary_object: "repair object",
}

func (o Repair_boundary) String() string {
	return rb[o]
}

const (
	Boundary_block  Repair_boundary = repair_block
	Boundary_object                 = repair_object
)

type Instance struct {
	debug   bool
	wg      *sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
	handle  norm_instance_handle
	nevent  norm_event
	sess_db refdb
}

// If wg is not nil, wg.Add()/wg.Done() will be called for the internal
// goroutine thus allowing for a clean shutdown.
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormCreateInstance
//
func Create_instance(wg *sync.WaitGroup) (r *Instance, err error) {
	lock.Lock()
	defer lock.Unlock()
	if wg == nil {
		wg = &sync.WaitGroup{}
	}
	return create_instance(wg)
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormDestroyInstance
//
func (o *Instance) Destroy() {
	lock.Lock()
	defer lock.Unlock()
	o.cancel()
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStopInstance
//
func (o *Instance) Stop_instance() {
	lock.Lock()
	defer lock.Unlock()
	o.stop()
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormRestartInstance
//
func (o *Instance) Restart_instance() bool {
	lock.Lock()
	defer lock.Unlock()
	return o.restart()
}

//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetCacheDirectory
//
func (o *Instance) Set_cache_directory(cache_path string) bool {
	lock.Lock()
	defer lock.Unlock()
	return o.set_cache_directory(cache_path)
}

// Set_debug applies to Instance and not libnorm.
//
func (o *Instance) Set_debug(debug bool) {
	lock.Lock()
	defer lock.Unlock()
	o.debug = debug
}

// Get_debug applies to Instance and not libnorm.
//
func (o *Instance) Get_debug() bool {
	lock.Lock()
	defer lock.Unlock()
	return o.debug
}

// debug_level: 3, set between 0 and 12 inclusive.
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetDebugLevel
//
func (o *Instance) Set_debug_level(debug_level uint) {
	lock.Lock()
	defer lock.Unlock()
	o.set_debug_level(debug_level)
}

// default: stderr
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormOpenDebugLog
//
func (o *Instance) Open_debug_log(file_name string) bool {
	lock.Lock()
	defer lock.Unlock()
	return o.open_debug_log(file_name)
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormCloseDebugLog
//
func (o *Instance) Close_debug_log() {
	lock.Lock()
	defer lock.Unlock()
	o.close_debug_log()
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormOpenDebugPipe
//
func (o *Instance) Open_debug_pipe(file_name string) bool {
	lock.Lock()
	defer lock.Unlock()
	return o.open_debug_pipe(file_name)
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormCloseDebugPipe
//
func (o *Instance) Close_debug_pipe() {
	lock.Lock()
	defer lock.Unlock()
	o.close_debug_pipe()
}

func (o *Instance) Get_debug_level() uint {
	lock.Lock()
	defer lock.Unlock()
	return o.get_debug_level()
}

func (o *Instance) Get_version() string {
	lock.Lock()
	defer lock.Unlock()
	return o.get_version()
}

func (o *Instance) sess() *Session {
	return o.sess_db[norm_session_handle(o.nevent.session)]
}

func (o *Instance) obj() *Object {
	return o.sess().objects[norm_object_handle(o.nevent.object)]
}

type Session struct {
	i              *Instance
	events         Event_type // bit mask
	c              chan *Event
	handle         norm_session_handle
	objects        map[norm_object_handle]*Object
	obj            *Object
	user_data_lock sync.Mutex
	user_data      interface{}
}

// node_id: 0, set to time.Now()
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormCreateSession
//
// https://raw.githubusercontent.com/aletheia7/norm/master/norm/NormSocketBindingNotes.txt
//
func (o *Instance) Create_session(address string, port int, node_id Node_id) (r *Session, err error) {
	lock.Lock()
	defer lock.Unlock()
	return o.create_session(address, port, node_id)
}

func (o *Session) Events() <-chan *Event {
	lock.Lock()
	defer lock.Unlock()
	return o.c
}

func (o *Session) Get_events() Event_type {
	lock.Lock()
	defer lock.Unlock()
	return o.events
}

// Set_events determines the events that are delivered through the
// Session.Events() channel. events is a bit flag.
// Default: Event_type_all
//
//  var sess Session
//  log.Println(sess.Get_events()) // Output: Event_type_all
//
//  // Event_type_all minus Event_type_grtt_updated minus Event_type_tx_object_sent
//  sess.Set_events(sess.Get_events() &^ (Event_type_grtt_updated | Event_type_tx_object_sent))
//
func (o *Session) Set_events(events Event_type) {
	lock.Lock()
	defer lock.Unlock()
	o.events = events
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormDestroySession
//
// Destroy also ends the internal goroutine.
//
func (o *Session) Destroy() {
	lock.Lock()
	defer lock.Unlock()
	o.destroy()
}

func (o *Session) Set_message_trace(trace bool) {
	lock.Lock()
	defer lock.Unlock()
	o.set_message_trace(trace)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormGetTxRate
//
func (o *Session) Get_tx_rate() float64 {
	lock.Lock()
	defer lock.Unlock()
	return o.get_tx_rate()
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetTxRate
//
func (o *Session) Set_tx_rate(tx_rate float64) {
	lock.Lock()
	defer lock.Unlock()
	o.set_tx_rate(tx_rate)
}

// Send func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetTxRateBounds
//
func (o *Session) Set_tx_rate_bounds(rate_min, rate_max float64) {
	lock.Lock()
	defer lock.Unlock()
	o.set_tx_rate_bounds(rate_min, rate_max)
}

// Sender func
//
// defaults: id: 0 = set to time.Now(), buffer_space: 0 = 1024 * 1024,
// block_size: 0 = 64, num_parity: 0 = 16.
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStartSender
//
func (o *Session) Start_sender(id uint32, buffer_space uint, segment_size, block_size, num_parity uint16, fec_id uint8) bool {
	lock.Lock()
	defer lock.Unlock()
	pid := norm_session_id(id)
	if pid == 0 {
		pid = norm_session_id(uint32(time.Now().Unix()))
	}
	pbuffer_space := buffer_space
	if pbuffer_space == 0 {
		pbuffer_space = 1024 * 1024
	}
	psegment_size := segment_size
	if psegment_size == 0 {
		psegment_size = 1400
	}
	pblock_size := block_size
	if pblock_size == 0 {
		pblock_size = 64
	}
	pnum_parity := num_parity
	if pnum_parity == 0 {
		pnum_parity = 16
	}
	return o.start_sender(pid, pbuffer_space, psegment_size, pblock_size, pnum_parity, fec_id)
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStopSender
//
func (o *Session) Stop_sender() {
	lock.Lock()
	defer lock.Unlock()
	o.stop_sender()
}

// Sender func
//
// enable: recommended is true.
// adjust_rate: recommended is true, false is experimental.
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetCongestionControl
//
func (o *Session) Set_congestion_control(enable, adjust_rate bool) {
	lock.Lock()
	defer lock.Unlock()
	o.set_congestion_control(enable, adjust_rate)
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetTxOnly
//
func (o *Session) Set_tx_only(tx_only, connect_to_session_address bool) {
	lock.Lock()
	defer lock.Unlock()
	o.set_tx_only(tx_only, connect_to_session_address)
}

// Sender func
//
// Data_enqueue does not copy data or info. Do not modify info and data until
// an Event_type_tx_object_purged is received for the returned *Object.Id.
// Object_type_data and Object_type_file objects are received in the
// Event_type_rx_object_completed Event.O.Data.
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormDataEnqueue
//
func (o *Session) Data_enqueue(data, info []byte) (*Object, error) {
	lock.Lock()
	defer lock.Unlock()
	return o.data_enqueue(data, info)
}

// Sender func
//
// Really only useful for silent non-nacking receivers.
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormRequeueObject
//
func (o *Session) Requeue_object(obj *Object) bool {
	lock.Lock()
	defer lock.Unlock()
	return o.requeue_object(obj)
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetLoopback
//
func (o *Session) Set_loopback(enable bool) {
	lock.Lock()
	defer lock.Unlock()
	o.set_loopback(enable)
}

// default: session port in Instance.Create_session().
//
// address can be nil. tx_port_reuse default: false.
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetTxPort
//
func (o *Session) Set_tx_port(tx_port uint16, tx_port_reuse bool, tx_bind_address string) bool {
	lock.Lock()
	defer lock.Unlock()
	return o.set_tx_port(tx_port, tx_port_reuse, tx_bind_address)
}

// Sender func
//
// defaults: size_max = 20 Mbyte, count_min = 8, recommended min = 2,
// count_max = 256
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetTxCacheBounds
//
func (o *Session) Set_tx_cache_bounds(size_max uint, count_min, count_max uint32) {
	lock.Lock()
	defer lock.Unlock()
	o.set_tx_cache_bounds(size_max, count_min, count_max)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormAddAckingNode
//
func (o *Session) Add_acking_node(node_id Node_id) bool {
	lock.Lock()
	defer lock.Unlock()
	return o.add_acking_node(node_id)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormRemoveAckingNode
//
func (o *Session) Add_remove_acking_node(node_id uint32) {
	lock.Lock()
	defer lock.Unlock()
	o.add_remove_acking_node(node_id)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormGetNextAckingNode
//
func (o *Session) Get_next_acking_node(status Acking_status) (node_id uint32, success bool) {
	lock.Lock()
	defer lock.Unlock()
	return o.get_next_acking_node(status)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormGetAckingStatus
//
func (o *Session) Get_acking_status(node_id uint32) Acking_status {
	lock.Lock()
	defer lock.Unlock()
	return o.get_acking_status(node_id)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetWatermark
//
func (o *Session) Set_watermark(obj *Object, override_flush bool) bool {
	lock.Lock()
	defer lock.Unlock()
	return o.set_watermark(obj, override_flush)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormCancelWatermark
//
func (o *Session) Cancel_watermark() {
	lock.Lock()
	defer lock.Unlock()
	o.cancel_watermark()
}

// Receiver func
//
// default: false
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetDefaultUnicastNack
//
func (o *Session) Set_default_unicast_nack(enable bool) {
	lock.Lock()
	defer lock.Unlock()
	o.set_default_unicast_nack(enable)
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetDefaultRepairBoundary
//
func (o *Session) Set_default_repair_boundary(boundary Repair_boundary) {
	lock.Lock()
	defer lock.Unlock()
	o.set_default_repair_boundary(boundary)
}

// rx_address: can be nil
//
// rx_send_address can be nil
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetRxPortReuse
//
func (o *Session) Set_rx_port_reuse(rx_port_reuse bool, rx_bind_address, rx_sender_address string, rx_sender_address_port uint16) {
	lock.Lock()
	defer lock.Unlock()
	o.set_rx_port_reuse(rx_port_reuse, rx_bind_address, rx_sender_address, rx_sender_address_port)
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStartReceiver
//
func (o *Session) Start_receiver(buffer uint) bool {
	lock.Lock()
	defer lock.Unlock()
	return o.start_receiver(buffer)
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStopReceiver
//
func (o *Session) Stop_receiver() {
	lock.Lock()
	defer lock.Unlock()
	o.stop_receiver()
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetSilentReceiver
//
func (o *Session) Set_silent_receiver(silent bool, max_delay int) {
	lock.Lock()
	defer lock.Unlock()
	o.set_silent_receiver(silent, max_delay)
}

// Receiver func
//
// Call before Start_receiver()
//
// default: 256, max: Max_rx_cache_limit
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetRxCacheLimit
//
func (o *Session) Set_rx_cache_limit(rx_cache_limit int) {
	lock.Lock()
	defer lock.Unlock()
	o.set_rx_cache_limit(rx_cache_limit)
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetDefaultSyncPolicy
//
func (o *Session) Set_default_sync_Policy(policy Sync_policy) {
	lock.Lock()
	defer lock.Unlock()
	o.set_default_sync_Policy(policy)
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetDefaultRxRobustFactor
//
func (o *Session) Set_default_rx_robust_factor(rx_robust_factor int) {
	lock.Lock()
	defer lock.Unlock()
	o.set_default_rx_robust_factor(rx_robust_factor)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStreamOpen
//
func (o *Session) Stream_open(buffer_size int, info []byte) (obj *Object, err error) {
	lock.Lock()
	defer lock.Unlock()
	return o.stream_open(buffer_size, info)
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetDefaultNackingMode
//
func (o *Session) Set_default_nacking_mode(mode Nacking_mode) {
	lock.Lock()
	defer lock.Unlock()
	o.set_default_nacking_mode(mode)
}

// Sender func
//
// Instance.Set_cache_directory() must be called in the Session Receiver in
// order for file transfers to occur.
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormFileEnqueue
//
func (o *Session) File_enqueue(file_name string, info []byte) (*Object, error) {
	lock.Lock()
	defer lock.Unlock()
	return o.file_enqueue(file_name, info)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSendCommand
//
func (o *Session) Send_command(cmd []byte, robust bool) bool {
	lock.Lock()
	defer lock.Unlock()
	return o.send_command(cmd, robust)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormCancelCommand
//
func (o *Session) Cancel_command() {
	lock.Lock()
	defer lock.Unlock()
	o.cancel_command()
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetTxSocketBuffer
//
func (o *Session) Set_tx_socket_buffer(buffer_size uint) bool {
	lock.Lock()
	defer lock.Unlock()
	return o.set_tx_socket_buffer(buffer_size)
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetFlowControl
//
func (o *Session) Set_flow_control(factor float64) {
	lock.Lock()
	defer lock.Unlock()
	o.set_flow_control(factor)
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetAutoParity
//
func (o *Session) Set_auto_parity(auto_parity uint8) {
	lock.Lock()
	defer lock.Unlock()
	o.set_auto_parity(auto_parity)
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetRxSocketBuffer
//
func (o *Session) Set_rx_socket_buffer(buffer_size uint) bool {
	lock.Lock()
	defer lock.Unlock()
	return o.set_rx_socket_buffer(buffer_size)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormGetLocalNodeId
//
func (o *Session) Get_local_node_id() Node_id {
	lock.Lock()
	defer lock.Unlock()
	return o.get_local_node_id()
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetMulticastInterface
//
func (o *Session) Set_multicast_interface(address string) bool {
	lock.Lock()
	defer lock.Unlock()
	return o.set_multicast_interface(address)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetSSM
//
func (o *Session) Set_ssm(address string) bool {
	lock.Lock()
	defer lock.Unlock()
	return o.set_ssm(address)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetTTL
//
func (o *Session) Set_ttl(ttl uint8) bool {
	lock.Lock()
	defer lock.Unlock()
	return o.set_ttl(ttl)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetTOS
//
func (o *Session) Set_tos(tos uint8) bool {
	lock.Lock()
	defer lock.Unlock()
	return o.set_tos(tos)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetFragmentation
//
func (o *Session) Set_fragmentation(enable bool) bool {
	lock.Lock()
	defer lock.Unlock()
	return o.set_fragmentation(enable)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormGetGrttEstimate
//
func (o *Session) Get_grtt_estimate() float64 {
	lock.Lock()
	defer lock.Unlock()
	return o.get_grtt_estimate()
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetGrttEstimate
//
func (o *Session) Set_grtt_estimate(grtt float64) {
	lock.Lock()
	defer lock.Unlock()
	o.set_grtt_estimate(grtt)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetGrttMax
//
func (o *Session) Set_grtt_max(max float64) {
	lock.Lock()
	defer lock.Unlock()
	o.set_grtt_max(max)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetGrttProbingMode
//
func (o *Session) Set_grtt_probing_mode(mode Probe_mode) {
	lock.Lock()
	defer lock.Unlock()
	o.set_grtt_probing_mode(mode)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetGrttProbingInterval
//
func (o *Session) Set_grtt_probing_interval(min, max float64) {
	lock.Lock()
	defer lock.Unlock()
	o.set_grtt_probing_interval(min, max)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetBackoffFactor
//
func (o *Session) Set_backoff_factor(factor float64) {
	lock.Lock()
	defer lock.Unlock()
	o.set_backoff_factor(factor)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetGroupSize
//
func (o *Session) Set_group_size(size uint) {
	lock.Lock()
	defer lock.Unlock()
	o.set_group_size(size)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetTxRobustFactor
//
func (o *Session) Set_tx_robust_factor(factor int) {
	lock.Lock()
	defer lock.Unlock()
	o.set_tx_robust_factor(factor)
}

// Sender func
//
func (o *Session) Get_user_data() interface{} {
	o.user_data_lock.Lock()
	defer o.user_data_lock.Unlock()
	return o.user_data
}

// Sender func
//
// Set_user_data stores data with the Session.
// Set_user_data does not store user_data in Norm.
//
func (o *Session) Set_user_data(user_data interface{}) {
	o.user_data_lock.Lock()
	defer o.user_data_lock.Unlock()
	o.user_data = user_data
}

func (o *Session) release(oh norm_object_handle) {
	if obj, ok := o.objects[oh]; ok {
		delete(o.objects, oh)
		if o.obj == obj {
			o.obj = nil
		}
	}
}

func (o *Session) release_all() {
	if o.obj != nil {
		o.obj = nil
	}
	o.objects = map[norm_object_handle]*Object{}
}

type Object struct {
	// sequential integer (1 based) per Instance
	Id uint64
	ot Object_type

	info *bytes.Buffer // Session.Data_enqueue()
	data *bytes.Buffer // Session.Data_enqueue()

	handle          norm_object_handle
	retained        bool
	stream_read_buf *bytes.Buffer
}

// Only Id and Type.
//
func (o Object) String() string {
	lock.Lock()
	defer lock.Unlock()
	return fmt.Sprintf("object id: %v, type: %v", o.Id, o.ot)
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormFileRename
//
func (o *Object) File_rename(new_file_name string) bool {
	lock.Lock()
	defer lock.Unlock()
	return o.file_rename(new_file_name)
}

func new_object(data, info []byte, t Object_type) *Object {
	r := &Object{
		ot:   t,
		data: bytes.NewBuffer(data),
		info: bytes.NewBuffer(info),
	}
	return r
}

func new_object_buf(data, info *bytes.Buffer, t Object_type) *Object {
	r := &Object{ot: t}
	r.set_info(info)
	r.set_data(data)
	return r
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormObjectGetType
//
func (o *Object) Get_type() Object_type {
	return o.ot
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormObjectHasInfo
//
func (o *Object) Has_info() bool {
	lock.Lock()
	defer lock.Unlock()
	return o.has_info()
}

func (o *Object) set_info(info *bytes.Buffer) {
	if info == nil {
		o.info = &bytes.Buffer{}
	} else {
		o.info = info
	}
}

func (o *Object) set_data(data *bytes.Buffer) {
	if data == nil {
		o.data = &bytes.Buffer{}
	} else {
		o.data = data
	}
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormObjectGetInfoLength
//
func (o *Object) Get_info_length() int {
	lock.Lock()
	defer lock.Unlock()
	return o.get_info_length()
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormObjectGetInfo
//
func (o *Object) Get_info() (info *bytes.Buffer) {
	lock.Lock()
	defer lock.Unlock()
	return o.get_info()
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormObjectGetSize
//
func (o *Object) Get_size() uint64 {
	lock.Lock()
	defer lock.Unlock()
	return o.get_size()
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormObjectGetBytesPending
//
func (o *Object) Get_bytes_pending() uint64 {
	lock.Lock()
	defer lock.Unlock()
	return o.get_bytes_pending()
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormObjectCancel
//
func (o *Object) Cancel() {
	lock.Lock()
	defer lock.Unlock()
	o.cancel()
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormObjectRetain
//
func (o *Object) Retain() {
	lock.Lock()
	defer lock.Unlock()
	if !o.retained {
		o.retain()
	}
}

func (o *Object) Is_retained() bool {
	lock.Lock()
	defer lock.Unlock()
	return o.retained
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormObjectRelease
//
func (o *Object) Release() {
	lock.Lock()
	defer lock.Unlock()
	if o.retained {
		o.release()
	}
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStreamRead
//
func (o *Object) Stream_read() (data *bytes.Buffer, ret bool) {
	lock.Lock()
	defer lock.Unlock()
	return o.stream_read()
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStreamWrite
//
func (o *Object) Stream_write(data []byte) int {
	lock.Lock()
	defer lock.Unlock()
	if o.ot != Object_type_stream || len(data) == 0 {
		return 0
	}
	return o.stream_write(data)
}

// Sender func
//
// graceful: false
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStreamClose
//
func (o *Object) Stream_close(graceful bool) {
	lock.Lock()
	defer lock.Unlock()
	o.stream_close(graceful)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStreamMarkEom
//
func (o *Object) Stream_mark_eom() {
	lock.Lock()
	defer lock.Unlock()
	o.stream_mark_eom()
}

type Flush_mode int

const (
	Flush_none    Flush_mode = flush_none
	Flush_passive            = flush_passive
	Flush_active             = flush_active
)

var fm = map[Flush_mode]string{
	Flush_none:    "flush none",
	Flush_passive: "flush passive",
	Flush_active:  "flush active",
}

func (o Flush_mode) String() string {
	return fm[o]
}

type Probe_mode int

const (
	Probe_none    Probe_mode = probe_none
	Probe_passive            = probe_passive
	Probe_active             = probe_active
)

var pm = map[Probe_mode]string{
	Probe_none:    "probe none",
	Probe_passive: "probe passive",
	Probe_active:  "probe active",
}

func (o Probe_mode) String() string {
	return pm[o]
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStreamFlush
//
func (o *Object) Stream_flush(eom bool, mode Flush_mode) {
	lock.Lock()
	defer lock.Unlock()
	o.stream_flush(eom, mode)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStreamSetAutoFlush
//
func (o *Object) Stream_set_auto_flush(mode Flush_mode) {
	lock.Lock()
	defer lock.Unlock()
	o.stream_set_auto_flush(mode)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStreamSetPushEnable
//
func (o *Object) Stream_set_push_enable(enable bool) {
	lock.Lock()
	defer lock.Unlock()
	o.stream_set_push_enable(enable)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStreamHasVacancy
//
func (o *Object) Stream_has_vacancy() bool {
	lock.Lock()
	defer lock.Unlock()
	return o.stream_has_vacancy()
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStreamSeekMsgStart
//
func (o *Object) Stream_seek_msg_start() bool {
	lock.Lock()
	defer lock.Unlock()
	return o.stream_seek_msg_start()
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStreamGetReadOffset
//
func (o *Object) Stream_get_read_offset() uint64 {
	lock.Lock()
	defer lock.Unlock()
	return o.stream_get_read_offset()
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormObjectSetNackingMode
//
func (o *Object) Set_nacking_mode(mode Nacking_mode) {
	lock.Lock()
	defer lock.Unlock()
	o.set_nacking_mode(mode)
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormFileGetName
//
func (o *Object) File_get_name() (file_name *bytes.Buffer) {
	lock.Lock()
	defer lock.Unlock()
	return o.file_get_name()
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormDataAccessData
//
func (o *Object) Data_access_data(release bool) (data *bytes.Buffer) {
	lock.Lock()
	defer lock.Unlock()
	return o.data_access_data(release)
}

// Use Data_access_data()
//
func (o *Object) Data_detach_data() {}

// Returns: *Node or nil
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormObjectGetSender
//
func (o *Object) Get_sender() (node *Node) {
	lock.Lock()
	defer lock.Unlock()
	return o.get_sender()
}

func (o *Object) destroy() {
	o.data.Reset()
	o.info.Reset()
	if o.stream_read_buf != nil {
		o.stream_read_buf.Reset()
	}
}

type Node struct {
	handle norm_node_handle
}

func new_node(handle norm_node_handle) *Node {
	return &Node{handle: handle}
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormNodeGetId
//
func (o *Node) Get_id() uint32 {
	lock.Lock()
	defer lock.Unlock()
	return o.get_id()
}

// Returns: address ("127.0.0.1") and port, or nil
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormNodeGetAddress
//
func (o *Node) Get_address() (address string, port uint16, success bool) {
	lock.Lock()
	defer lock.Unlock()
	return o.get_address()
}

// Returns: address:port ("127.0.0.1:32233")  or nil
//
func (o *Node) Get_address_all() string {
	lock.Lock()
	defer lock.Unlock()
	address, port, success := o.get_address()
	if success {
		return fmt.Sprintf("%v:%v", address, port)
	}
	return ``
}

// Returns: cmd will contain the command or be empty and non-nil.
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormNodeGetCommand
//
func (o *Node) Get_command() (cmd *bytes.Buffer) {
	lock.Lock()
	defer lock.Unlock()
	return o.get_command()
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormNodeSetUnicastNack
//
func (o *Node) Set_unicast_nack(enable bool) {
	lock.Lock()
	defer lock.Unlock()
	o.set_unicast_nack(enable)
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormNodeSetNackingMode
//
func (o *Node) Set_nacking_mode(mode Nacking_mode) {
	lock.Lock()
	defer lock.Unlock()
	o.set_nacking_mode(mode)
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetRepairBoundary
//
func (o *Node) Set_repair_boundary(boundary Repair_boundary) {
	lock.Lock()
	defer lock.Unlock()
	o.set_repair_boundary(boundary)
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormNodeGetGrtt
//
func (o *Node) Get_grtt() float64 {
	lock.Lock()
	defer lock.Unlock()
	return o.get_grtt()
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormNodeFreeBuffers
//
func (o *Node) Free_buffers() {
	lock.Lock()
	defer lock.Unlock()
	o.free_buffers()
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormNodeDelete
//
func (o *Node) Delete() {
	lock.Lock()
	defer lock.Unlock()
	o.delete()
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormNodeRetain
//
func (o *Node) Retain() {
	lock.Lock()
	defer lock.Unlock()
	o.retain()
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormNodeRelease
//
func (o *Node) Release() {
	lock.Lock()
	defer lock.Unlock()
	o.release()
}

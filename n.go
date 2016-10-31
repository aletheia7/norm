// Copyright 2016 aletheia7. All rights reserved. Use of this source code is
// governed by a BSD-2-Clause license that can be found in the LICENSE file.

//go:generate go build go-norm-build/norm-build.go
//go:generate norm-build
//go:generate rm norm-build

package norm

import (
	"bytes"
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
	panic(fmt.Sprintf("missing Object_type: %v", o))
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
	return fmt.Sprintf("node %x", o)
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
	debug        bool
	wg           *sync.WaitGroup
	handle       norm_instance_handle
	cmdc         chan interface{}
	nevent       norm_event
	sess_db      refdb
	object_id_ct uint64
	debug_level  uint
	version      string
	cache_path   string
	file_name    string
	*cmd_mgr
}

// If wg is not nil, wg.Add()/wg.Done() will be called for the internal
// goroutine thus allowing for a clean shutdown.
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormCreateInstance
//
func Create_instance(wg *sync.WaitGroup) (r *Instance, err error) {
	instance_lock.Lock()
	defer instance_lock.Unlock()
	if wg == nil {
		wg = &sync.WaitGroup{}
	}
	return create_instance(wg)
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormDestroyInstance
//
func (o *Instance) Destroy() error {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.icmdc() <- icmd_destroy_instance
	e := <-o.errc
	o.handle = nil
	return e
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStopInstance
//
func (o *Instance) Stop_instance() {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.icmdc() <- icmd_stop_instance
	<-o.errc
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormRestartInstance
//
func (o *Instance) Restart_instance() bool {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.icmdc() <- icmd_restart_instance
	if nil == <-o.errc {
		return true
	}
	return false
}

//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetCacheDirectory
//
func (o *Instance) Set_cache_directory(cache_path string) bool {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cache_path = cache_path
	o.icmdc() <- icmd_set_cache_directory
	if nil == <-o.errc {
		return true
	}
	return false
}

// Set_debug applies to Instance and not libnorm.
//
func (o *Instance) Set_debug(debug bool) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.debug = debug
}

// Get_debug applies to Instance and not libnorm.
//
func (o *Instance) Get_debug() bool {
	o.lock.Lock()
	defer o.lock.Unlock()
	return o.debug
}

// debug_level: 3, set between 0 and 12 inclusive.
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetDebugLevel
//
func (o *Instance) Set_debug_level(debug_level uint) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.debug_level = debug_level
	o.icmdc() <- icmd_set_debug_level
	<-o.errc
}

// default: stderr
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormOpenDebugLog
//
func (o *Instance) Open_debug_log(file_name string) bool {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.file_name = file_name
	o.icmdc() <- icmd_open_debug_log
	if nil == <-o.errc {
		return true
	}
	return false
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormCloseDebugLog
//
func (o *Instance) Close_debug_log() {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.icmdc() <- icmd_close_debug_log
	<-o.errc
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormOpenDebugPipe
//
func (o *Instance) Open_debug_pipe(file_name string) bool {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.file_name = file_name
	o.icmdc() <- icmd_open_debug_pipe
	if nil == <-o.errc {
		return true
	}
	return false
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormCloseDebugPipe
//
func (o *Instance) Close_debug_pipe() {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.icmdc() <- icmd_close_debug_pipe
	<-o.errc
}

func (o *Instance) Get_debug_level() uint {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.icmdc() <- icmd_get_debug_level
	<-o.errc
	return o.debug_level
}

func (o *Instance) Get_version() string {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.icmdc() <- icmd_get_version
	<-o.errc
	return o.version
}

func (o *Instance) sess() *Session {
	return o.sess_db[norm_session_handle(o.nevent.session)]
}

func (o *Instance) obj() *Object {
	return o.sess().objects[norm_object_handle(o.nevent.object)]
}

func (o Instance) icmdc() chan<- interface{} {
	return o.cmdc
}

type cmd_mgr struct {
	cmd   icmd
	icmdc func() chan<- interface{}
	errc  chan error
	lock  sync.Mutex
}

func new_cmd_mgr(icmdc func() chan<- interface{}) *cmd_mgr {
	return &cmd_mgr{icmdc: icmdc, errc: make(chan error, 1)}
}

type Session struct {
	events                               Event_type // bit mask
	c                                    chan *Event
	enable, enable2                      bool
	bind_address, sender_address         string
	port                                 uint16
	i                                    int
	size                                 uint
	segment_size, block_size, num_parity uint16 // Start_sender()
	handle                               norm_session_handle
	node_id                              norm_node_id
	acking_status                        Acking_status
	sync_policy                          Sync_policy
	nacking_mode                         Nacking_mode
	repair_boundary                      Repair_boundary
	tx_cache_bounds_size_max             uint
	tx_cache_bounds_count_min            uint32
	tx_cache_bounds_count_max            uint32
	ttl_tos                              uint8
	double, double2                      float64
	probe_mode                           Probe_mode
	file_name                            string
	u8                                   uint8
	buf                                  *bytes.Buffer
	id                                   norm_session_id
	objects                              map[norm_object_handle]*Object
	obj                                  *Object
	written                              int
	*cmd_mgr
}

// node_id: 0, set to time.Now()
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormCreateSession
//
// https://raw.githubusercontent.com/aletheia7/norm/master/norm/NormSocketBindingNotes.txt
//
func (o *Instance) Create_session(address string, port uint16, node_id Node_id) (r *Session, err error) {
	o.lock.Lock()
	defer o.lock.Unlock()
	r = &Session{
		c:            make(chan *Event, 100),
		events:       Event_type_all,
		objects:      map[norm_object_handle]*Object{},
		bind_address: address,
		port:         port,
	}
	r.cmd_mgr = new_cmd_mgr(o.icmdc)
	r.cmd = icmd_create_session
	if node_id == 0 {
		r.node_id = norm_node_id(uint32(time.Now().Unix()))
	} else {
		r.node_id = norm_node_id(node_id)
	}
	r.icmdc() <- r
	if nil != <-r.errc {
		r = nil
	}
	return
}

func (o *Session) Events() <-chan *Event {
	o.lock.Lock()
	defer o.lock.Unlock()
	return o.c
}

func (o *Session) Get_events() Event_type {
	o.lock.Lock()
	defer o.lock.Unlock()
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
	o.lock.Lock()
	defer o.lock.Unlock()
	o.events = events
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormDestroySession
//
// Destroy also ends the internal goroutine.
//
func (o *Session) Destroy() {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_destroy_session
	o.icmdc() <- o
	<-o.errc
}

func (o *Session) Set_message_trace(trace bool) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.enable = trace
	o.cmd = icmd_set_message_trace
	o.icmdc() <- o
	<-o.errc
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormGetTxRate
//
func (o *Session) Get_tx_rate() float64 {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_get_tx_rate
	o.icmdc() <- o
	<-o.errc
	return o.double
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetTxRate
//
func (o *Session) Set_tx_rate(tx_rate float64) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.double = tx_rate
	o.cmd = icmd_set_tx_rate
	o.icmdc() <- o
	<-o.errc
}

// Send func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetTxRateBounds
//
func (o *Session) Set_tx_rate_bounds(rate_min, rate_max float64) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.double = rate_min
	o.double2 = rate_max
	o.cmd = icmd_set_tx_rate_bounds
	o.icmdc() <- o
	<-o.errc
}

// Sender func
//
// defaults: id: 0 = set to time.Now(), buffer_space: 0 = 1024 * 1024,
// block_size: 0 = 64, num_parity: 0 = 16.
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStartSender
//
func (o *Session) Start_sender(id uint32, buffer_space uint, segment_size, block_size, num_parity uint16, fec_id uint8) error {
	o.lock.Lock()
	defer o.lock.Unlock()
	if id == 0 {
		o.id = norm_session_id(uint32(time.Now().Unix()))
	} else {
		o.id = norm_session_id(id)
	}
	if buffer_space == 0 {
		o.size = 1024 * 1024
	} else {
		o.size = buffer_space
	}
	if segment_size == 0 {
		o.segment_size = 1400
	} else {
		o.segment_size = segment_size
	}
	if block_size == 0 {
		o.block_size = 64
	} else {
		o.block_size = block_size
	}
	if num_parity == 0 {
		o.num_parity = 16
	} else {
		o.num_parity = num_parity
	}
	o.u8 = fec_id
	o.cmd = icmd_start_sender
	o.icmdc() <- o
	return <-o.errc
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStopSender
//
func (o *Session) Stop_sender() {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_stop_sender
	o.icmdc() <- o
	<-o.errc
	return
}

// Sender func
//
// enable: recommended is true.
// adjust_rate: recommended is true, false is experimental.
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetCongestionControl
//
func (o *Session) Set_congestion_control(enable, adjust_rate bool) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.enable = enable
	o.enable2 = adjust_rate
	o.cmd = icmd_set_congestion_control
	o.icmdc() <- o
	<-o.errc
	return
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetTxOnly
//
func (o *Session) Set_tx_only(tx_only, connect_to_session_address bool) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.enable = tx_only
	o.enable2 = connect_to_session_address
	o.cmd = icmd_set_tx_only
	o.icmdc() <- o
	<-o.errc
	return
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
	o.lock.Lock()
	defer o.lock.Unlock()
	o.obj = new_object(o.icmdc, data, info, Object_type_data)
	if int(o.segment_size) < o.obj.info.Len() {
		o.release(o.obj.handle)
		return nil, fmt.Errorf("info size exeeds segment size: %v > %v", o.obj.info.Len(), o.segment_size)
	}
	o.cmd = icmd_data_enqueue
	o.icmdc() <- o
	return o.obj, <-o.errc
}

// Sender func
//
// Really only useful for silent non-nacking receivers.
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormRequeueObject
//
func (o *Session) Requeue_object(obj *Object) error {
	o.lock.Lock()
	defer o.lock.Unlock()
	switch obj.ot {
	case Object_type_data, Object_type_file:
	default:
		return E_false
	}
	o.cmd = icmd_object_requeue
	o.obj = obj
	o.icmdc() <- o
	return <-o.errc
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetLoopback
//
func (o *Session) Set_loopback(enable bool) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.enable = enable
	o.cmd = icmd_set_loopback
	o.icmdc() <- o
	<-o.errc
}

// default: session port in Instance.Create_session().
//
// address can be nil. tx_port_reuse default: false.
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetTxPort
//
func (o *Session) Set_tx_port(tx_port uint16, tx_port_reuse bool, tx_bind_address string) error {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.bind_address = tx_bind_address
	o.port = tx_port
	o.enable = tx_port_reuse
	o.cmd = icmd_set_tx_port
	o.icmdc() <- o
	return <-o.errc
}

// Sender func
//
// defaults: size_max = 20 Mbyte, count_min = 8, recommended min = 2,
// count_max = 256
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetTxCacheBounds
//
func (o *Session) Set_tx_cache_bounds(size_max uint, count_min, count_max uint32) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.tx_cache_bounds_size_max = size_max
	o.tx_cache_bounds_count_min = count_min
	o.tx_cache_bounds_count_max = count_max
	o.cmd = icmd_set_tx_cache_bounds
	o.icmdc() <- o
	<-o.errc
	return
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormAddAckingNode
//
func (o *Session) Add_acking_node(node_id Node_id) error {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.node_id = norm_node_id(node_id)
	o.cmd = icmd_add_acking_node
	o.icmdc() <- o
	return <-o.errc
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormRemoveAckingNode
//
func (o *Session) Add_remove_acking_node(node_id uint32) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.node_id = norm_node_id(node_id)
	o.cmd = icmd_remove_acking_node
	o.icmdc() <- o
	<-o.errc
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormGetNextAckingNode
//
func (o *Session) Get_next_acking_node(status Acking_status) (node_id uint32, success bool) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.node_id = 0
	o.cmd = icmd_get_next_acking_node
	o.icmdc() <- o
	if nil == <-o.errc {
		return uint32(o.node_id), true
	}
	return uint32(o.node_id), false
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormGetAckingStatus
//
func (o *Session) Get_acking_status(node_id uint32) Acking_status {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.node_id = norm_node_id(node_id)
	o.cmd = icmd_get_acking_status
	<-o.errc
	return o.acking_status
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetWatermark
//
func (o *Session) Set_watermark(obj *Object, override_flush bool) bool {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.enable = override_flush
	o.cmd = icmd_set_watermark
	o.icmdc() <- o
	if nil == <-o.errc {
		return true
	}
	return false
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormCancelWatermark
//
func (o *Session) Cancel_watermark() {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_cancel_watermark
	o.icmdc() <- o
	<-o.errc
}

// Receiver func
//
// default: false
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetDefaultUnicastNack
//
func (o *Session) Set_default_unicast_nack(enable bool) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.enable = enable
	o.cmd = icmd_set_default_unicast_nack
	o.icmdc() <- o
	<-o.errc
	return
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetDefaultRepairBoundary
//
func (o *Session) Set_default_repair_boundary(boundary Repair_boundary) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.repair_boundary = boundary
	o.cmd = icmd_set_default_repair_boundary
	o.icmdc() <- o
	<-o.errc
	return
}

// rx_address: can be nil
//
// rx_send_address can be nil
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetRxPortReuse
//
func (o *Session) Set_rx_port_reuse(rx_port_reuse bool, rx_bind_address, rx_sender_address string, rx_sender_address_port uint16) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.enable = rx_port_reuse
	o.bind_address = rx_bind_address
	o.sender_address = rx_sender_address
	o.port = rx_sender_address_port
	o.cmd = icmd_set_rx_port_reuse
	o.icmdc() <- o
	<-o.errc
	return
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStartReceiver
//
func (o *Session) Start_receiver(buffer uint) error {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.size = buffer
	o.cmd = icmd_start_receiver
	o.icmdc() <- o
	return <-o.errc
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStopReceiver
//
func (o *Session) Stop_receiver() {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_stop_receiver
	o.icmdc() <- o
	<-o.errc
	return
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetSilentReceiver
//
func (o *Session) Set_silent_receiver(silent bool, max_delay int) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.enable = silent
	o.i = max_delay
	o.cmd = icmd_set_silent_receiver
	o.icmdc() <- o
	<-o.errc
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
	o.lock.Lock()
	defer o.lock.Unlock()
	if rx_cache_limit < 0 || Max_rx_cache_limit < rx_cache_limit {
		return
	}
	o.i = rx_cache_limit
	o.cmd = icmd_set_rx_cache_limit
	o.icmdc() <- o
	<-o.errc
	return
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetDefaultSyncPolicy
//
func (o *Session) Set_default_sync_Policy(policy Sync_policy) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.sync_policy = policy
	o.cmd = icmd_set_default_sync_policy
	o.icmdc() <- o
	<-o.errc
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetDefaultRxRobustFactor
//
func (o *Session) Set_default_rx_robust_factor(rx_robust_factor int) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.i = rx_robust_factor
	o.cmd = icmd_set_default_rx_robust_factor
	o.icmdc() <- o
	<-o.errc
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStreamOpen
//
func (o *Session) Stream_open(buffer_size int, info []byte) (obj *Object, err error) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.obj = new_object(o.icmdc, nil, info, Object_type_stream)
	if int(o.segment_size) < o.obj.info.Len() {
		o.release(o.obj.handle)
		return nil, fmt.Errorf("info size exeeds segment size: %v > %v", o.obj.info.Len(), o.segment_size)
	}
	o.cmd = icmd_stream_open
	o.icmdc() <- o
	if e := <-o.errc; e != nil {
		o.release(o.obj.handle)
		return nil, e
	}
	return o.obj, nil
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetDefaultNackingMode
//
func (o *Session) Set_default_nacking_mode(mode Nacking_mode) {
	o.cmd = icmd_set_default_nacking_mode
	o.nacking_mode = mode
	o.icmdc() <- o
	<-o.errc
}

// Sender func
//
// Instance.Set_cache_directory() must be called in the Session Receiver in
// order for file transfers to occur.
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormFileEnqueue
//
func (o *Session) File_enqueue(file_name string, info []byte) (*Object, error) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.obj = new_object(o.icmdc, nil, info, Object_type_file)
	if int(o.segment_size) < o.obj.info.Len() {
		o.release(o.obj.handle)
		return nil, fmt.Errorf("info size exeeds segment size: %v > %v", o.obj.info.Len(), o.segment_size)
	}
	o.file_name = file_name
	o.cmd = icmd_file_enqueue
	o.icmdc() <- o
	return o.obj, <-o.errc
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSendCommand
//
func (o *Session) Send_command(cmd []byte, robust bool) bool {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.buf = bytes.NewBuffer(cmd)
	o.enable = robust
	o.cmd = icmd_send_command
	o.icmdc() <- o
	if nil == <-o.errc {
		return true
	}
	return false
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormCancelCommand
//
func (o *Session) Cancel_command(cmd []byte) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_cancel_command
	o.icmdc() <- o
	<-o.errc
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetTxSocketBuffer
//
func (o *Session) Set_tx_socket_buffer(buffer_size uint) bool {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.size = buffer_size
	o.cmd = icmd_set_tx_socket_buffer
	o.icmdc() <- o
	if nil == <-o.errc {
		return true
	}
	return false
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetFlowControl
//
func (o *Session) Set_flow_control(factor float64) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.double = factor
	o.cmd = icmd_set_flow_control
	o.icmdc() <- o
	<-o.errc
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetAutoParity
//
func (o *Session) Set_auto_parity(auto_parity uint8) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.u8 = auto_parity
	o.cmd = icmd_set_auto_parity
	o.icmdc() <- o
	<-o.errc
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetRxSocketBuffer
//
func (o *Session) Set_rx_socket_buffer(buffer_size uint) bool {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.size = buffer_size
	o.cmd = icmd_set_rx_socket_buffer
	o.icmdc() <- o
	if nil == <-o.errc {
		return true
	}
	return false
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormGetLocalNodeId
//
func (o *Session) Get_local_node_id() uint32 {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_get_local_node_id
	o.icmdc() <- o
	<-o.errc
	return uint32(o.node_id)
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetMulticastInterface
//
func (o *Session) Set_multicast_interface(address string) bool {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.bind_address = address
	o.cmd = icmd_set_multicast_interface
	o.icmdc() <- o
	if nil == <-o.errc {
		return true
	}
	return false
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetSSM
//
func (o *Session) Set_ssm(address string) bool {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.bind_address = address
	o.cmd = icmd_set_ssm
	o.icmdc() <- o
	if nil == <-o.errc {
		return true
	}
	return false
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetTTL
//
func (o *Session) Set_ttl(ttl uint8) bool {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.ttl_tos = ttl
	o.cmd = icmd_set_ttl
	o.icmdc() <- o
	if nil == <-o.errc {
		return true
	}
	return false
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetTOS
//
func (o *Session) Set_tos(tos uint8) bool {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.ttl_tos = tos
	o.cmd = icmd_set_tos
	o.icmdc() <- o
	if nil == <-o.errc {
		return true
	}
	return false
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetFragmentation
//
func (o *Session) Set_fragmentation(enable bool) bool {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.enable = enable
	o.cmd = icmd_set_fragmentation
	o.icmdc() <- o
	if nil == <-o.errc {
		return true
	}
	return false
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormGetGrttEstimate
//
func (o *Session) Get_grtt_estimate() float64 {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_get_grtt_estimate
	o.icmdc() <- o
	<-o.errc
	return o.double
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetGrttEstimate
//
func (o *Session) Set_grtt_estimate(grtt float64) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.double = grtt
	o.cmd = icmd_set_grtt_estimate
	o.icmdc() <- o
	<-o.errc
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetGrttMax
//
func (o *Session) Set_grtt_max(max float64) float64 {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.double = max
	o.cmd = icmd_set_grtt_max
	o.icmdc() <- o
	<-o.errc
	return o.double
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetGrttProbingMode
//
func (o *Session) Set_grtt_probing_mode(mode Probe_mode) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.probe_mode = mode
	o.cmd = icmd_set_grtt_probing_mode
	o.icmdc() <- o
	<-o.errc
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetGrttProbingInterval
//
func (o *Session) Set_grtt_probing_interval(min, max float64) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.double = min
	o.double2 = max
	o.cmd = icmd_set_grtt_probing_interval
	o.icmdc() <- o
	<-o.errc
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetBackoffFactor
//
func (o *Session) Set_backoff_factor(factor float64) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.double = factor
	o.cmd = icmd_set_backoff_factor
	o.icmdc() <- o
	<-o.errc
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetGroupSize
//
func (o *Session) Set_group_size(size uint) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.size = size
	o.cmd = icmd_set_group_size
	o.icmdc() <- o
	<-o.errc
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetTxRobustFactor
//
func (o *Session) Set_tx_robust_factor(factor int) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.i = factor
	o.cmd = icmd_set_tx_robust_factor
	o.icmdc() <- o
	<-o.errc
}

// Sender func
//
// todo
//
func (o *Session) Get_user_data() {}

// Sender func
//
// todo
//
func (o *Session) Set_user_data() {}

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

	handle                   norm_object_handle
	data_access_data_release bool
	buf                      *bytes.Buffer
	size, offset             uint64
	written                  int
	i                        int
	retained                 bool
	nacking_mode             Nacking_mode
	node                     *Node
	file_name                string
	stream_read_buf          *bytes.Buffer
	flush_mode               Flush_mode // Stream_flush()
	enable                   bool
	*cmd_mgr
}

// Only Id and Type.
//
func (o Object) String() string {
	o.lock.Lock()
	defer o.lock.Unlock()
	return fmt.Sprintf("object id: %v, type: %v", o.Id, o.ot)
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormFileRename
//
func (o *Object) File_rename(new_file_name string) bool {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.file_name = new_file_name
	o.cmd = icmd_file_rename
	o.icmdc() <- o
	e := <-o.errc
	if e == nil {
		return true
	}
	return false
}

func new_object(icmdc func() chan<- interface{}, data, info []byte, t Object_type) *Object {
	r := &Object{
		ot:   t,
		data: bytes.NewBuffer(data),
		info: bytes.NewBuffer(info),
	}
	r.cmd_mgr = new_cmd_mgr(icmdc)
	return r
}

func new_object_buf(icmdc func() chan<- interface{}, data, info *bytes.Buffer, t Object_type) *Object {
	r := &Object{ot: t}
	r.cmd_mgr = new_cmd_mgr(icmdc)
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
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_object_has_info
	o.icmdc() <- o
	if nil == <-o.errc {
		return true
	}
	return false
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
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_object_get_info_length
	o.icmdc() <- o
	<-o.errc
	return o.i
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormObjectGetInfo
//
func (o *Object) Get_info() (info *bytes.Buffer) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_object_get_info
	o.icmdc() <- o
	<-o.errc
	info = o.buf
	o.buf = nil
	return
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormObjectGetSize
//
func (o *Object) Get_size() uint64 {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_object_get_size
	o.icmdc() <- o
	<-o.errc
	return o.size
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormObjectGetBytesPending
//
func (o *Object) Get_bytes_pending() uint64 {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_object_get_bytes_pending
	o.icmdc() <- o
	<-o.errc
	return o.size
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormObjectCancel
//
func (o *Object) Cancel() {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_object_cancel
	o.icmdc() <- o
	<-o.errc
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormObjectRetain
//
func (o *Object) Retain() {
	o.lock.Lock()
	defer o.lock.Unlock()
	if o.retained {
		return
	}
	o.cmd = icmd_object_retain
	o.icmdc() <- o
	<-o.errc
}

func (o *Object) Is_retained() bool {
	o.lock.Lock()
	defer o.lock.Unlock()
	return o.retained
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormObjectRelease
//
func (o *Object) Release() {
	o.lock.Lock()
	defer o.lock.Unlock()
	if !o.retained {
		return
	}
	o.cmd = icmd_object_release
	o.icmdc() <- o
	<-o.errc
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStreamRead
//
func (o *Object) Stream_read() (data *bytes.Buffer, ret bool) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_stream_read
	o.icmdc() <- o
	if e := <-o.errc; e == nil {
		ret = true
	}
	data = o.buf
	o.buf = nil
	return
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStreamWrite
//
func (o *Object) Stream_write(data []byte) int {
	o.lock.Lock()
	defer o.lock.Unlock()
	if o.ot != Object_type_stream || len(data) == 0 {
		return 0
	}
	o.buf = bytes.NewBuffer(data)
	o.cmd = icmd_stream_write
	o.icmdc() <- o
	<-o.errc
	o.buf = nil
	return o.written
}

// Sender func
//
// graceful: false
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStreamClose
//
func (o *Object) Stream_close(graceful bool) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.enable = graceful
	o.cmd = icmd_stream_close
	o.icmdc() <- o
	<-o.errc
	return
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStreamMarkEom
//
func (o *Object) Stream_mark_eom() {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_stream_mark_eom
	o.icmdc() <- o
	<-o.errc
	return
}

// todo
type Flush_mode int

const (
	Flush_none    Flush_mode = flush_none
	Flush_passive            = flush_passive
	Flush_active             = flush_active
)

// todo
type Probe_mode int

const (
	Probe_none    Probe_mode = probe_none
	Probe_passive            = probe_passive
	Probe_active             = probe_active
)

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStreamFlush
//
func (o *Object) Stream_flush(eom bool, mode Flush_mode) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.enable = eom
	o.flush_mode = mode
	o.cmd = icmd_stream_flush
	o.icmdc() <- o
	<-o.errc
	return
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStreamSetAutoFlush
//
func (o *Object) Stream_set_auto_flush(mode Flush_mode) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.flush_mode = mode
	o.cmd = icmd_stream_set_auto_flush
	o.icmdc() <- o
	<-o.errc
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStreamSetPushEnable
//
func (o *Object) Stream_set_push_enable(enable bool) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.enable = enable
	o.cmd = icmd_stream_set_push_enable
	o.icmdc() <- o
	<-o.errc
}

// Sender func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStreamHasVacancy
//
func (o *Object) Stream_has_vacancy() bool {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_stream_has_vacancy
	o.icmdc() <- o
	if nil == <-o.errc {
		return true
	}
	return false
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStreamSeekMsgStart
//
func (o *Object) Stream_seek_msg_start() bool {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_stream_seek_msg_start
	o.icmdc() <- o
	if e := <-o.errc; e == nil {
		return true
	}
	return false
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormStreamGetReadOffset
//
func (o *Object) Stream_get_read_offset() uint64 {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_stream_get_read_offset
	o.icmdc() <- o
	<-o.errc
	return o.offset
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormObjectSetNackingMode
//
func (o *Object) Set_nacking_mode(mode Nacking_mode) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.nacking_mode = mode
	o.cmd = icmd_object_set_nacking_mode
	o.icmdc() <- o
	<-o.errc
	return
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormFileGetName
//
func (o *Object) File_get_name() (file_name *bytes.Buffer) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_file_get_name
	o.icmdc() <- o
	<-o.errc
	file_name = o.buf
	o.buf = nil
	return
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormDataAccessData
//
func (o *Object) Data_access_data(release bool) (data *bytes.Buffer) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.data_access_data_release = release
	o.cmd = icmd_data_access_data
	o.icmdc() <- o
	<-o.errc
	data = o.buf
	o.buf = nil
	return
}

// Use Data_access_data()
//
func (o *Object) Data_detach_data() {}

// Returns: *Node or nil
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormObjectGetSender
//
func (o *Object) Get_sender() (node *Node) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_object_get_sender
	o.icmdc() <- o
	<-o.errc
	node = o.node
	o.node = nil
	return
}

func (o *Object) destroy() {
	o.data.Reset()
	o.info.Reset()
	if o.stream_read_buf != nil {
		o.stream_read_buf.Reset()
	}
}

type Node struct {
	id              uint32
	f64             float64
	handle          norm_node_handle
	address         string
	port            uint16
	command         *bytes.Buffer
	enable          bool
	nacking_mode    Nacking_mode
	repair_boundary Repair_boundary
	*cmd_mgr
}

func new_node(icmdc func() chan<- interface{}, handle norm_node_handle) *Node {
	r := &Node{handle: handle}
	r.cmd_mgr = new_cmd_mgr(icmdc)
	return r
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormNodeGetId
//
func (o *Node) Get_id() uint32 {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_node_get_id
	o.icmdc() <- o
	<-o.errc
	return o.id
}

// Returns: address ("127.0.0.1") and port, or nil
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormNodeGetAddress
//
func (o *Node) Get_address() (address string, port uint16, success bool) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_node_get_address
	o.icmdc() <- o
	e := <-o.errc
	if e == nil {
		return o.address, o.port, true
	}
	return o.address, o.port, false
}

// Returns: address:port ("127.0.0.1:32233")  or nil
//
func (o *Node) Get_address_all() string {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_node_get_address
	o.icmdc() <- o
	e := <-o.errc
	if e == nil {
		return fmt.Sprintf("%v:%v", o.address, o.port)
	}
	return ``
}

// Returns: cmd will contain the command or be empty and non-nil.
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormNodeGetCommand
//
func (o *Node) Get_command() (cmd *bytes.Buffer) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_node_get_command
	o.icmdc() <- o
	<-o.errc
	cmd = o.command
	o.command = nil
	return cmd
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormNodeSetUnicastNack
//
func (o *Node) Set_unicast_nack(enable bool) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.enable = enable
	o.cmd = icmd_node_set_unicast_nack
	o.icmdc() <- o
	<-o.errc
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormNodeSetNackingMode
//
func (o *Node) Set_nacking_mode(mode Nacking_mode) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.nacking_mode = mode
	o.cmd = icmd_node_set_nacking_mode
	o.icmdc() <- o
	<-o.errc
}

// Receiver func
//
// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormSetRepairBoundary
//
func (o *Node) Set_repair_boundary(boundary Repair_boundary) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.repair_boundary = boundary
	o.cmd = icmd_node_set_repair_boundary
	o.icmdc() <- o
	<-o.errc
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormNodeGetGrtt
//
func (o *Node) Get_grtt() float64 {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_node_get_grtt
	o.icmdc() <- o
	<-o.errc
	return o.f64
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormNodeFreeBuffers
//
func (o *Node) Free_buffers() {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_node_free_buffers
	o.icmdc() <- o
	<-o.errc
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormNodeDelete
//
func (o *Node) Delete() {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_node_delete
	o.icmdc() <- o
	<-o.errc
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormNodeRetain
//
func (o *Node) Retain() {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_node_retain
	o.icmdc() <- o
	<-o.errc
}

// https://htmlpreview.github.io/?https://github.com/aletheia7/norm/blob/master/norm/doc/NormDeveloperGuide.html#NormNodeRelease
//
func (o *Node) Release() {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.cmd = icmd_node_release
	o.icmdc() <- o
	<-o.errc
}

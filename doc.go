// Copyright 2016 aletheia7. All rights reserved. Use of this source code is
// governed by a BSD-2-Clause license that can be found in the LICENSE file.

/*
Package norm provides reliable UDP using multicast and unicast
sockets. norm is a cgo wrapper for NACK-Oriented Reliable Multicast (NORM).
libnorm can replace TCP/IP, and in many use cases, provide greater
performance.

The goal of this package is to provide a one-to-one functional feature set
with libnorm. The main exception is the Instance object and
NormGetNextEvent(). Instance will create one goroutine to talk to libnorm.
Multiple Instance objects can be used when necessary. In addition, multiple
Session objects can be used under one Instance.

The maximum Object_type_data size is limited to the largest []byte indexed by
an int type. The maximum []byte on a 32 bit system is ~ 2 GB (math.MaxInt32).
The maximum []byte on a 64 bit system is much greater (math.MaxInt64). One
can send larger data than an int type by sending multiple Object_type_data
messages, or using an Object_type_file, or Object_type_stream. This package
could easily be extended to support larger Object_type_data types. Make a
github.com issue if this is necessary.

The Object_type_file and Object_type_data data types are delivered reliably,
however, objects can be delivered out-of-order. Object_type_stream is
delivered reliably and bytes arrive in order, however, the application
determines the message boundaries for a group of bytes.

The Object_type_data and Object_type_file data types will be retained
(Object.Retain()/(NormObjectRetain()) automatically. The application must
call Object.Release() to free memory when these Object_types are no longer
used.

Package norm is safe for concurrent goroutine use.
*/
package norm

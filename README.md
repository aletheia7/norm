[![](https://img.shields.io/badge/godoc-reference-blue.svg)](https://godoc.org/github.com/aletheia7/norm) 

===
[![](http://www.nrl.navy.mil/itd/ncs/sites/www.nrl.navy.mil.itd.ncs/files/field/image/NormLogo.gif)](http://www.nrl.navy.mil/itd/ncs/products/norm)

[NACK-Oriented Reliable Multicast (NORM)](http://www.nrl.navy.mil/itd/ncs/products/norm)

===

#### Install

1. norm/protolib C++ libraries are included. 
1. `go get -d github.com/aletheia7/norm`
1. `cd $GOPATH/src/github.com/aletheia7/norm` 
1. `go generate`

#### Examples

##### data_send_recv (normDataSend/normDataRecv) #####
  Ready to run on one machine at 127.0.0.1:6003/6004
```sh
cd examples/data_send_recv 
go install
data_send_recv -h
data_send_recv -r &
data_send_recv -s
```
##### stream_send_recv (normStreamSend/normStreamRecv) #####
  Ready to run on one machine at 127.0.0.1:6003/6004
```sh
cd examples/stream_send_recv 
go install
stream_send_recv -h
stream_send_recv -r &
stream_send_recv -s
```
##### stream_send_recv (normFileSend/normFileRecv) #####
  Ready to run on one machine at 127.0.0.1:6003/6004
```sh
cd examples/file_send_recv 
go install
file_send_recv -h
file_send_recv -r &
file_send_recv -s
```
#### NORM Support

  - [NORM Users and Developers List](http://pf.itd.nrl.navy.mil/mailman/listinfo/norm-dev)
  - [List Archive](http://pf.itd.nrl.navy.mil/pipermail/norm-dev/)

#### License 

Use of this source code is governed by a BSD-2-Clause license that can be found
in the LICENSE file.

[![BSD-2-Clause License](img/osi_logo_100X133_90ppi_0.png)]
(https://opensource.org/)

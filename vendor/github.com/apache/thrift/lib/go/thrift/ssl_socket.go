/*
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements. See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership. The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License. You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package thrift

import (
	"context"
	"crypto/tls"
	"net"
	"time"
)

type TSSLSocket struct {
	conn *socketConn
	// hostPort contains host:port (e.g. "asdf.com:12345"). The field is
	// only valid if addr is nil.
	hostPort string
	// addr is nil when hostPort is not "", and is only used when the
	// TSSLSocket is constructed from a net.Addr.
	addr net.Addr
	cfg  *tls.Config

	connectTimeout time.Duration
	socketTimeout  time.Duration
}

// NewTSSLSocket creates a net.Conn-backed TTransport, given a host and port and tls Configuration
//
// Example:
// 	trans, err := thrift.NewTSSLSocket("localhost:9090", nil)
func NewTSSLSocket(hostPort string, cfg *tls.Config) (*TSSLSocket, error) {
	return NewTSSLSocketTimeout(hostPort, cfg, 0, 0)
}

// NewTSSLSocketTimeout creates a net.Conn-backed TTransport, given a host and port
// it also accepts a tls Configuration and connect/socket timeouts as time.Duration
func NewTSSLSocketTimeout(hostPort string, cfg *tls.Config, connectTimeout, socketTimeout time.Duration) (*TSSLSocket, error) {
	if cfg.MinVersion == 0 {
		cfg.MinVersion = tls.VersionTLS10
	}
	return &TSSLSocket{
		hostPort:       hostPort,
		cfg:            cfg,
		connectTimeout: connectTimeout,
		socketTimeout:  socketTimeout,
	}, nil
}

// Creates a TSSLSocket from a net.Addr
func NewTSSLSocketFromAddrTimeout(addr net.Addr, cfg *tls.Config, connectTimeout, socketTimeout time.Duration) *TSSLSocket {
	return &TSSLSocket{
		addr:           addr,
		cfg:            cfg,
		connectTimeout: connectTimeout,
		socketTimeout:  socketTimeout,
	}
}

// Creates a TSSLSocket from an existing net.Conn
func NewTSSLSocketFromConnTimeout(conn net.Conn, cfg *tls.Config, socketTimeout time.Duration) *TSSLSocket {
	return &TSSLSocket{
		conn:          wrapSocketConn(conn),
		addr:          conn.RemoteAddr(),
		cfg:           cfg,
		socketTimeout: socketTimeout,
	}
}

// Sets the connect timeout
func (p *TSSLSocket) SetConnTimeout(timeout time.Duration) error {
	p.connectTimeout = timeout
	return nil
}

// Sets the socket timeout
func (p *TSSLSocket) SetSocketTimeout(timeout time.Duration) error {
	p.socketTimeout = timeout
	return nil
}

func (p *TSSLSocket) pushDeadline(read, write bool) {
	var t time.Time
	if p.socketTimeout > 0 {
		t = time.Now().Add(time.Duration(p.socketTimeout))
	}
	if read && write {
		p.conn.SetDeadline(t)
	} else if read {
		p.conn.SetReadDeadline(t)
	} else if write {
		p.conn.SetWriteDeadline(t)
	}
}

// Connects the socket, creating a new socket object if necessary.
func (p *TSSLSocket) Open() error {
	var err error
	// If we have a hostname, we need to pass the hostname to tls.Dial for
	// certificate hostname checks.
	if p.hostPort != "" {
		if p.conn, err = createSocketConnFromReturn(tls.DialWithDialer(
			&net.Dialer{
				Timeout: p.connectTimeout,
			},
			"tcp",
			p.hostPort,
			p.cfg,
		)); err != nil {
			return NewTTransportException(NOT_OPEN, err.Error())
		}
	} else {
		if p.conn.isValid() {
			return NewTTransportException(ALREADY_OPEN, "Socket already connected.")
		}
		if p.addr == nil {
			return NewTTransportException(NOT_OPEN, "Cannot open nil address.")
		}
		if len(p.addr.Network()) == 0 {
			return NewTTransportException(NOT_OPEN, "Cannot open bad network name.")
		}
		if len(p.addr.String()) == 0 {
			return NewTTransportException(NOT_OPEN, "Cannot open bad address.")
		}
		if p.conn, err = createSocketConnFromReturn(tls.DialWithDialer(
			&net.Dialer{
				Timeout: p.connectTimeout,
			},
			p.addr.Network(),
			p.addr.String(),
			p.cfg,
		)); err != nil {
			return NewTTransportException(NOT_OPEN, err.Error())
		}
	}
	return nil
}

// Retrieve the underlying net.Conn
func (p *TSSLSocket) Conn() net.Conn {
	return p.conn
}

// Returns true if the connection is open
func (p *TSSLSocket) IsOpen() bool {
	return p.conn.IsOpen()
}

// Closes the socket.
func (p *TSSLSocket) Close() error {
	// Close the socket
	if p.conn != nil {
		err := p.conn.Close()
		if err != nil {
			return err
		}
		p.conn = nil
	}
	return nil
}

func (p *TSSLSocket) Read(buf []byte) (int, error) {
	if !p.conn.isValid() {
		return 0, NewTTransportException(NOT_OPEN, "Connection not open")
	}
	p.pushDeadline(true, false)
	// NOTE: Calling any of p.IsOpen, p.conn.read0, or p.conn.IsOpen between
	// p.pushDeadline and p.conn.Read could cause the deadline set inside
	// p.pushDeadline being reset, thus need to be avoided.
	n, err := p.conn.Read(buf)
	return n, NewTTransportExceptionFromError(err)
}

func (p *TSSLSocket) Write(buf []byte) (int, error) {
	if !p.conn.isValid() {
		return 0, NewTTransportException(NOT_OPEN, "Connection not open")
	}
	p.pushDeadline(false, true)
	return p.conn.Write(buf)
}

func (p *TSSLSocket) Flush(ctx context.Context) error {
	return nil
}

func (p *TSSLSocket) Interrupt() error {
	if !p.conn.isValid() {
		return nil
	}
	return p.conn.Close()
}

func (p *TSSLSocket) RemainingBytes() (num_bytes uint64) {
	const maxSize = ^uint64(0)
	return maxSize // the truth is, we just don't know unless framed is used
}

package main

import (
	"net"
)

type IPBlockList struct {
	ips     map[string]struct{}
	subnets []*net.IPNet
}

func newIPBlockList(entries []string) *IPBlockList {
	bl := &IPBlockList{ips: make(map[string]struct{})}
	for _, e := range entries {
		if _, subnet, err := net.ParseCIDR(e); err == nil {
			bl.subnets = append(bl.subnets, subnet)
		} else if ip := net.ParseIP(e); ip != nil {
			bl.ips[ip.String()] = struct{}{}
		}
	}
	return bl
}

func (bl *IPBlockList) isBlocked(addr string) bool {
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	if _, ok := bl.ips[ip.String()]; ok {
		return true
	}
	for _, subnet := range bl.subnets {
		if subnet.Contains(ip) {
			return true
		}
	}
	return false
}

func wrapListener(l net.Listener, bl *IPBlockList) net.Listener {
	if bl == nil || (len(bl.ips) == 0 && len(bl.subnets) == 0) {
		return l
	}
	return &blockingListener{Listener: l, bl: bl}
}

type blockingListener struct {
	net.Listener
	bl *IPBlockList
}

func (l *blockingListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		host, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
		if l.bl.isBlocked(host) {
			conn.Close()
			continue
		}
		return conn, nil
	}
}

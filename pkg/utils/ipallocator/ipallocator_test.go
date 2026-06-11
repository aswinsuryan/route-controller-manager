/*
Copyright 2015 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ipallocator

import (
	"net"
	"testing"
)

func parseCIDR(t *testing.T, cidr string) *net.IPNet {
	t.Helper()
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatalf("unexpected error parsing CIDR %q: %v", cidr, err)
	}
	return ipNet
}

func TestNewInMemory(t *testing.T) {
	r, err := NewInMemory(parseCIDR(t, "172.16.0.0/28"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// /28 = 16 addresses, minus network and broadcast = 14 usable
	if r.Free() != 14 {
		t.Errorf("expected 14 free addresses, got %d", r.Free())
	}
}

func TestAllocateSpecific(t *testing.T) {
	r, _ := NewInMemory(parseCIDR(t, "172.16.0.0/28"))
	ip := net.ParseIP("172.16.0.1")

	if err := r.Allocate(ip); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Has(ip) {
		t.Error("expected ip to be allocated")
	}
}

func TestAllocateNext(t *testing.T) {
	r, _ := NewInMemory(parseCIDR(t, "172.16.0.0/28"))
	freeBefore := r.Free()

	ip, err := r.AllocateNext()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip == nil {
		t.Fatal("expected non-nil IP")
	}
	if !r.Has(ip) {
		t.Error("allocated IP should be marked as in use")
	}
	if r.Free() != freeBefore-1 {
		t.Errorf("expected %d free, got %d", freeBefore-1, r.Free())
	}
}

func TestRelease(t *testing.T) {
	r, _ := NewInMemory(parseCIDR(t, "172.16.0.0/28"))
	ip := net.ParseIP("172.16.0.1")

	r.Allocate(ip)
	freeBefore := r.Free()

	if err := r.Release(ip); err != nil {
		t.Fatalf("unexpected error releasing: %v", err)
	}
	if r.Has(ip) {
		t.Error("expected ip to be released")
	}
	if r.Free() != freeBefore+1 {
		t.Errorf("expected %d free, got %d", freeBefore+1, r.Free())
	}
}

func TestReleaseOutOfRange(t *testing.T) {
	r, _ := NewInMemory(parseCIDR(t, "172.16.0.0/28"))
	if err := r.Release(net.ParseIP("10.0.0.1")); err != nil {
		t.Errorf("releasing out-of-range IP should be a no-op, got error: %v", err)
	}
}

func TestErrNotInRange(t *testing.T) {
	r, _ := NewInMemory(parseCIDR(t, "172.16.0.0/28"))

	err := r.Allocate(net.ParseIP("10.0.0.1"))
	if err == nil {
		t.Fatal("expected error for out-of-range IP")
	}
	if _, ok := err.(*ErrNotInRange); !ok {
		t.Errorf("expected *ErrNotInRange, got %T: %v", err, err)
	}
}

func TestErrAllocated(t *testing.T) {
	r, _ := NewInMemory(parseCIDR(t, "172.16.0.0/28"))
	ip := net.ParseIP("172.16.0.1")

	r.Allocate(ip)
	err := r.Allocate(ip)
	if err != ErrAllocated {
		t.Errorf("expected ErrAllocated, got %v", err)
	}
}

func TestErrFull(t *testing.T) {
	// /30 = 4 addresses, minus network and broadcast = 2 usable
	r, _ := NewInMemory(parseCIDR(t, "172.16.0.0/30"))
	total := r.Free()
	for i := 0; i < total; i++ {
		if _, err := r.AllocateNext(); err != nil {
			t.Fatalf("unexpected error on allocation %d: %v", i, err)
		}
	}
	_, err := r.AllocateNext()
	if err != ErrFull {
		t.Errorf("expected ErrFull, got %v", err)
	}
}

func TestHasNotAllocated(t *testing.T) {
	r, _ := NewInMemory(parseCIDR(t, "172.16.0.0/28"))
	if r.Has(net.ParseIP("172.16.0.1")) {
		t.Error("expected Has to return false for unallocated IP")
	}
}

func TestHasOutOfRange(t *testing.T) {
	r, _ := NewInMemory(parseCIDR(t, "172.16.0.0/28"))
	if r.Has(net.ParseIP("10.0.0.1")) {
		t.Error("expected Has to return false for out-of-range IP")
	}
}

func TestAllocateAndReleaseAll(t *testing.T) {
	r, _ := NewInMemory(parseCIDR(t, "172.16.0.0/28"))
	total := r.Free()
	ips := make([]net.IP, 0, total)

	for i := 0; i < total; i++ {
		ip, err := r.AllocateNext()
		if err != nil {
			t.Fatalf("unexpected error on allocation %d: %v", i, err)
		}
		ips = append(ips, ip)
	}
	if r.Free() != 0 {
		t.Errorf("expected 0 free, got %d", r.Free())
	}

	for _, ip := range ips {
		if err := r.Release(ip); err != nil {
			t.Fatalf("unexpected error releasing %v: %v", ip, err)
		}
	}
	if r.Free() != total {
		t.Errorf("expected %d free after releasing all, got %d", total, r.Free())
	}
}
